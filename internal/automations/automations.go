// Package automations is the one place an Automation's definition is stored,
// validated and its lifecycle changed (ADR 0016). /site and /api handlers stay thin
// adapters over it; the step executor (internal/jobs) decodes the same Step format,
// so the storage JSON has a single definition. The definition (Trigger, Integration and
// steps) lives in versions (ADR 0049): an edit writes the draft, Publish makes it live
// (ADR 0065).
package automations

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Step kinds.
const (
	StepEmail     = "email"
	StepWait      = "wait"
	StepApplyTag  = "apply_tag"
	StepRemoveTag = "remove_tag"
	StepHTTP      = "http"
)

// Step is one node of an Automation's linear sequence, in the executor's on-disk
// JSON form. Email steps carry Subject and Body (HTML), wait steps Seconds, tag
// steps Tag (the Tag name).
type Step struct {
	Type    string `json:"type"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
	// Doc is the editor's JSON document an email step's Body was built from, and EditorVersion
	// the editor that rendered it (ADR 0040). A step written as HTML has neither. A step being
	// saved carries a Doc or a Body, not both: the save renders the Doc to the Body.
	Doc           json.RawMessage `json:"doc,omitempty"`
	EditorVersion string          `json:"editorVersion,omitempty"`
	Seconds       int             `json:"seconds,omitempty"`
	Tag           string          `json:"tag,omitempty"`

	// HTTP steps (ADR 0039) hold ID, URL, Headers, Body (a Liquid JSON template) and
	// Secret. Stored, URL, header values and Secret are ciphertext, sealed per field;
	// in a step being saved, an empty one keeps the stored value of the step with the
	// same ID, and ClearSecret drops the signing secret.
	ID          string   `json:"id,omitempty"`
	URL         string   `json:"url,omitempty"`
	Headers     []Header `json:"headers,omitempty"`
	Secret      string   `json:"secret,omitempty"`
	ClearSecret bool     `json:"-"`
}

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Automation in this Workspace.
	ErrNotFound = validation.NotFound("automations: automation not found", "errors.automation_not_found")
	// ErrInvalidStep: a step has an unknown type, a negative wait, or a tag step
	// without a tag name, or an email step whose content cannot render.
	ErrInvalidStep = validation.Refuse("automations: invalid step", "errors.automation_step_invalid")
	// ErrInvalidTrigger: the trigger is an infrastructure Event (a send or a delivery notice)
	// that no Automation may start from (ADR 0053).
	ErrInvalidTrigger = validation.Refuse("automations: invalid trigger", "errors.automation_trigger_invalid")
	// ErrIntegrationNotFound: the Integration does not exist in this Workspace or is archived.
	ErrIntegrationNotFound = validation.Refuse("automations: integration not found", "errors.automation_integration_not_found")

	// ErrNothingToPublish: the Automation has neither a draft nor a live version.
	ErrNothingToPublish = validation.Refuse("automations: nothing to publish", "errors.automation_nothing_to_publish")
	// ErrNothingToRevert: the Automation has no draft above a live version. A never-published
	// Automation's draft is its whole definition, so it is not reverted either.
	ErrNothingToRevert = validation.Refuse("automations: nothing to revert", "errors.automation_nothing_to_revert")

	// ErrNoVersion: the Automation has no live version: it was never published, so nobody can
	// be enrolled in it.
	ErrNoVersion = errors.New("automations: automation has no live version")

	errBodyAndDoc = errors.New("automations: a step writes a body or a doc, not both")
)

// Decode parses a stored definition. An empty definition has no steps.
func Decode(definition string) ([]Step, error) {
	if definition == "" {
		return nil, nil
	}
	var steps []Step
	if err := json.Unmarshal([]byte(definition), &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// Encode serializes steps into the stored definition, dropping the fields that do
// not belong to each step's type, trimming tag names and sanitizing an email step's body.
func Encode(steps []Step) (string, error) {
	stored := make([]Step, 0, len(steps))
	for i := range steps {
		s := &steps[i]
		switch s.Type {
		case StepWait:
			stored = append(stored, Step{Type: StepWait, Seconds: s.Seconds})
		case StepApplyTag, StepRemoveTag:
			stored = append(stored, Step{Type: s.Type, Tag: strings.TrimSpace(s.Tag)})
		case StepHTTP:
			stored = append(stored, Step{Type: StepHTTP, ID: s.ID, URL: s.URL, Headers: s.Headers, Body: s.Body, Secret: s.Secret})
		default:
			email := Step{Type: StepEmail, Subject: s.Subject, Body: emailrender.SanitizeHTML(s.Body)}
			if len(s.Doc) > 0 {
				email.Doc, email.EditorVersion = s.Doc, s.EditorVersion
			}
			stored = append(stored, email)
		}
	}
	b, err := json.Marshal(stored)
	return string(b), err
}

func validate(steps []Step) error {
	for i := range steps {
		s := &steps[i]
		switch s.Type {
		case StepEmail:
			if err := emailrender.Lint(s.Subject, s.Body, nil).Error; err != nil {
				return ErrInvalidStep.With(err.Error())
			}
		case StepHTTP:
		case StepWait:
			if s.Seconds < 0 {
				return ErrInvalidStep
			}
		case StepApplyTag, StepRemoveTag:
			if strings.TrimSpace(s.Tag) == "" {
				return ErrInvalidStep
			}
		default:
			return ErrInvalidStep
		}
	}
	return nil
}

// Module is the automations module.
type Module struct {
	cipher   *secrets.Cipher
	bus      *events.Bus
	renderer *emaildoc.Renderer
}

// New builds the automations module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017). The cipher seals an HTTP step's secrets; the bus
// opens the transaction that saves an Automation together with its steps; the renderer builds an
// email step's body from its document.
func New(cipher *secrets.Cipher, bus *events.Bus, renderer *emaildoc.Renderer) *Module {
	return &Module{cipher: cipher, bus: bus, renderer: renderer}
}

// build renders every email step that holds a document to its body, in the same save (ADR 0040,
// the Template rules): a step with a document and a body is refused, and a document that cannot
// be rendered refuses the whole save, naming the step (1-based). A step with no document is
// left as it is, so a body-only write stores no document.
func (m *Module) build(ctx context.Context, steps []Step) ([]Step, error) {
	out := make([]Step, len(steps))
	copy(out, steps)
	for i := range out {
		s := out[i]
		if s.Type != StepEmail || len(s.Doc) == 0 {
			continue
		}
		var body *string
		if s.Body != "" {
			body = &s.Body
		}
		c, err := emailrender.ResolveContent(ctx, m.renderer, &s.Subject, body, s.Doc, errBodyAndDoc)
		switch {
		case errors.Is(err, errBodyAndDoc):
			return nil, ErrInvalidStep.With(fmt.Sprintf("step %d: write a body or a doc, not both", i+1))
		case errors.Is(err, emailrender.ErrInvalidContent):
			return nil, ErrInvalidStep.With(fmt.Sprintf("step %d: %s", i+1, err))
		case err != nil:
			return nil, err
		}
		out[i].Body, out[i].EditorVersion = c.Authored.Body, c.Authored.EditorVersion
	}
	return out, nil
}

// engagementTriggers are the `email.` Events an Automation may start from: what a recipient does.
// The others (sent, delivered, delayed, dropped, bounced, complained) are infrastructure, kept
// for the figures and the log (ADR 0053).
var engagementTriggers = []string{events.NameEmailOpened, events.NameEmailClicked, events.NameEmailUnsubscribed}

// validateTrigger refuses an empty trigger and an infrastructure Event as the trigger.
func validateTrigger(trigger string) error {
	t := strings.ToLower(strings.TrimSpace(trigger))
	if t == "" {
		return ErrInvalidTrigger.With("the trigger is empty")
	}
	if strings.HasPrefix(t, "email.") && !slices.Contains(engagementTriggers, t) {
		return ErrInvalidTrigger.With(fmt.Sprintf("%q is not a trigger; use %s", trigger, strings.Join(engagementTriggers, ", ")))
	}
	return nil
}

// CreateInput is the data for a new Automation. It is always created inactive: publishing is a
// separate, send-class operation (ADR 0065).
type CreateInput struct {
	Name         string
	TriggerEvent string
	Steps        []Step
	// IntegrationID is the email Integration the steps send through; nil is the Workspace default.
	IntegrationID *int64
}

// UpdateInput changes an Automation; nil fields are left alone. The name is the Automation's own;
// the rest is the definition, written to the draft (ADR 0065).
type UpdateInput struct {
	Name         *string
	TriggerEvent *string
	Steps        *[]Step
	// IntegrationID moves the Automation to that Integration; ClearIntegration goes back to
	// the Workspace default. Neither leaves it alone.
	IntegrationID    *int64
	ClearIntegration bool
}

// LiveAndDraft returns the Automation's live version and its draft, each nil when it has none.
// The Automations this module returns carry both.
func LiveAndDraft(a *ent.Automation) (live, draft *ent.AutomationVersion) {
	return a.Edges.LiveVersion, a.Edges.DraftVersion
}

// withVersions loads the live version and the draft of the Automations a query returns.
func withVersions(q *ent.AutomationQuery) *ent.AutomationQuery {
	return q.WithLiveVersion().WithDraftVersion()
}

// UsingTag returns the Workspace's kept Automations whose live version or draft applies or
// removes the Tag by its name, in id order (spec #767, "In use"). An archived Automation takes no
// part in new work.
func UsingTag(ctx context.Context, s *ent.Scoped, name string) ([]*ent.Automation, error) {
	all, err := withVersions(s.Automation().Query()).Order(ent.Asc(automation.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var using []*ent.Automation
	for _, a := range all {
		live, draft := LiveAndDraft(a)
		inLive, err := namesTag(live, name)
		if err != nil {
			return nil, err
		}
		inDraft, err := namesTag(draft, name)
		if err != nil {
			return nil, err
		}
		if inLive || inDraft {
			using = append(using, a)
		}
	}
	return using, nil
}

// namesTag reports whether a step of the version, if there is one, applies or removes the Tag.
func namesTag(v *ent.AutomationVersion, name string) (bool, error) {
	if v == nil {
		return false, nil
	}
	steps, err := Decode(v.Steps)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(steps, func(st Step) bool {
		return (st.Type == StepApplyTag || st.Type == StepRemoveTag) && strings.TrimSpace(st.Tag) == name
	}), nil
}

// List returns one page of the Workspace's Automations, newest first: the kept ones, or with
// archived the archived ones.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Automation], error) {
	rows := func() *ent.AutomationQuery {
		if archived {
			return s.Automation().QueryDiscarded()
		}
		return s.Automation().Query()
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Automation, error) {
			return withVersions(rows()).Order(ent.Desc(automation.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Get returns one Automation of the Workspace, archived or not: an archived Automation is
// still addressable (viewed, restored, its Enrollments listed).
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	a, err := withVersions(s.Automation().QueryWithDiscarded().Where(automation.ID(id))).Only(ctx)
	return a, notFound(err)
}

// Create stores a new inactive Automation whose first version is its draft: nothing is live until
// it is published (ADR 0065).
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Automation, error) {
	if err := validateTrigger(in.TriggerEvent); err != nil {
		return nil, err
	}
	built, err := m.build(ctx, in.Steps)
	if err != nil {
		return nil, err
	}
	in.Steps = built
	if err := validate(in.Steps); err != nil {
		return nil, err
	}
	if err := checkIntegration(ctx, s, in.IntegrationID); err != nil {
		return nil, err
	}
	def := "[]"
	if in.Steps != nil {
		sealed, err := m.sealSteps(in.Steps, nil)
		if err != nil {
			return nil, err
		}
		if def, err = Encode(sealed); err != nil {
			return nil, err
		}
	}
	var id int64
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		a, err := ts.Automation().Create().
			SetName(in.Name).
			SetStatus(automation.StatusInactive).
			Save(ctx)
		if err != nil {
			return err
		}
		id = a.ID
		v, err := ts.AutomationVersion().Create().
			SetAutomationID(a.ID).
			SetVersion(1).
			SetTriggerEvent(in.TriggerEvent).
			SetNillableIntegrationID(in.IntegrationID).
			SetSteps(def).
			Save(ctx)
		if err != nil {
			return err
		}
		return ts.Automation().UpdateOneID(a.ID).SetDraftVersionID(v.ID).Exec(ctx)
	})
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Update renames the Automation and writes its definition to the draft (ADR 0065): the fields
// left out keep the draft's, or the live version's when there is no draft. The live version is
// never rewritten: the first edit above it creates the draft, the next ones rewrite the draft in
// place. A definition equal to the draft's (or, with no draft, the live version's) stores nothing.
// The status is untouched.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Automation, error) {
	if in.TriggerEvent != nil {
		if err := validateTrigger(*in.TriggerEvent); err != nil {
			return nil, err
		}
	}
	var steps []Step
	if in.Steps != nil {
		built, err := m.build(ctx, *in.Steps)
		if err != nil {
			return nil, err
		}
		if err := validate(built); err != nil {
			return nil, err
		}
		steps = built
	}
	if err := checkIntegration(ctx, s, in.IntegrationID); err != nil {
		return nil, err
	}
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		a, err := m.lock(ctx, ts, id)
		if err != nil {
			return err
		}
		if in.Name != nil && *in.Name != a.Name {
			if err := ts.Automation().UpdateOneID(id).SetName(*in.Name).Exec(ctx); err != nil {
				return err
			}
		}
		return m.saveDraft(ctx, ts, a, in, steps)
	})
	if err != nil {
		return nil, notFound(err)
	}
	return m.Get(ctx, s, id)
}

// lock takes the Automation row for the rest of the transaction, archived or not, and returns it
// with its live version and draft: Enroll reads the live version under a share lock, so it waits
// for a Publish or an edit in flight, and two edits of one draft take turns.
func (m *Module) lock(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	if _, err := s.Automation().QueryWithDiscarded().Where(automation.ID(id)).ForUpdate().OnlyID(ctx); err != nil {
		return nil, notFound(err)
	}
	return m.Get(ctx, s, id)
}

// saveDraft merges the edit onto the draft, or onto the live version when there is none, and
// stores the result as the draft.
func (m *Module) saveDraft(ctx context.Context, s *ent.Scoped, a *ent.Automation, in UpdateInput, steps []Step) error {
	live, draft := LiveAndDraft(a)
	base := cmp.Or(draft, live)
	if base == nil {
		return ErrNoVersion
	}
	trigger := base.TriggerEvent
	if in.TriggerEvent != nil {
		trigger = *in.TriggerEvent
	}
	integrationID := base.IntegrationID
	switch {
	case in.ClearIntegration:
		integrationID = nil
	case in.IntegrationID != nil:
		integrationID = in.IntegrationID
	}
	stored, err := Decode(base.Steps)
	if err != nil {
		return err
	}
	// The stored steps in the form Encode writes them, so that an unchanged edit compares equal
	// whatever wrote the stored JSON.
	unchanged, err := Encode(stored)
	if err != nil {
		return err
	}
	def := unchanged
	if in.Steps != nil {
		sealed, err := m.sealSteps(steps, stored)
		if err != nil {
			return err
		}
		if def, err = Encode(sealed); err != nil {
			return err
		}
	}
	sameIntegration := equalIDs(integrationID, base.IntegrationID)
	if trigger == base.TriggerEvent && sameIntegration && def == unchanged {
		return nil
	}
	if draft != nil {
		upd := s.AutomationVersion().UpdateOneID(draft.ID).
			SetTriggerEvent(trigger).
			SetSteps(def).
			SetNillableIntegrationID(integrationID)
		if integrationID == nil {
			upd.ClearIntegrationID()
		}
		return upd.Exec(ctx)
	}
	newest, err := s.AutomationVersion().Query().
		Where(automationversion.AutomationID(a.ID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		First(ctx)
	if err != nil {
		return err
	}
	created, err := s.AutomationVersion().Create().
		SetAutomationID(a.ID).
		SetVersion(newest.Version + 1).
		SetTriggerEvent(trigger).
		SetNillableIntegrationID(integrationID).
		SetSteps(def).
		Save(ctx)
	if err != nil {
		return err
	}
	return s.Automation().UpdateOneID(a.ID).SetDraftVersionID(created.ID).Exec(ctx)
}

func equalIDs(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// checkIntegration refuses an Integration that is missing or archived, the rule a Broadcast
// save applies too (messaging.IntegrationKept). A live version already pointing at one keeps it
// as history.
func checkIntegration(ctx context.Context, s *ent.Scoped, id *int64) error {
	if id == nil {
		return nil
	}
	ok, err := messaging.IntegrationKept(ctx, s, *id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrIntegrationNotFound
	}
	return nil
}

// Delete purges an Automation that has no Enrollments and has sent nothing, with its versions
// (ADR 0049). One with any Enrollment or Outbound message is history and is archived instead: an
// *inuse.Error carrying their counts. The messages count apart from the Enrollments, because
// Contact erasure removes a Contact's Enrollments and keeps the mail they sent.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	if err := refuseWhileEnrolledByRules(ctx, s, id); err != nil {
		return err
	}
	n, err := s.Automation().Purge().
		Where(automation.ID(id), automation.Not(automation.HasRuns()), automation.Not(automation.HasOutboundMessages())).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	a, err := m.Get(ctx, s, id)
	if err != nil {
		return err
	}
	enrollments, err := s.AutomationRun().Query().Where(automationrun.AutomationID(id)).Count(ctx)
	if err != nil {
		return err
	}
	messages, err := s.OutboundMessage().Query().Where(outboundmessage.AutomationID(id)).Count(ctx)
	if err != nil {
		return err
	}
	var users []inuse.User
	if enrollments > 0 {
		users = append(users, inuse.Enrollments(id, a.Name, enrollments))
	}
	if messages > 0 {
		users = append(users, inuse.Messages(id, a.Name, messages))
	}
	if len(users) == 0 {
		return fmt.Errorf("automation %d: purge matched no row, yet it has no Enrollment and sent nothing", id)
	}
	return &inuse.Error{What: inuse.SubjectAutomation, Users: users}
}

// Archive discards an Automation (ADR 0049): it leaves the lists and enrolls nobody, and its
// Enrollments stay. An Automation with active Enrollments is refused with an *inuse.Error
// carrying their count. Archiving an archived Automation changes nothing.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) error {
	a, err := m.Get(ctx, s, id)
	if err != nil {
		return err
	}
	if a.DiscardedAt != nil {
		return nil
	}
	if err := refuseWhileEnrolledByRules(ctx, s, id); err != nil {
		return err
	}
	active, err := s.AutomationRun().Query().
		Where(automationrun.AutomationID(id), automationrun.StatusEQ(automationrun.StatusActive)).
		Count(ctx)
	if err != nil {
		return err
	}
	if active > 0 {
		return &inuse.Error{What: inuse.SubjectAutomation, Users: []inuse.User{inuse.Enrollments(id, a.Name, active)}}
	}
	return notFound(s.Automation().Discard(ctx, id))
}

// Unarchive brings an archived Automation back to the lists. Restoring one that is not
// archived changes nothing.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) error {
	return notFound(s.Automation().Restore(ctx, id))
}

// Publish makes the draft live and the Automation active: new Enrollments begin on the draft's
// definition, and the old live version stays as history; the Enrollments already on it finish
// there. With no draft it makes an inactive live version active again and leaves an active one as
// it is; with neither it is refused. The draft's Trigger is checked again and its Integration must
// still be kept. Publish is send-class (ADR 0065): the surfaces gate it by the publish scope. An
// archived Automation enrolls nobody and is restored before it is published, so it is as good as
// absent here: ErrNotFound.
func (m *Module) Publish(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		a, err := m.lock(ctx, ts, id)
		if err != nil {
			return err
		}
		if a.DiscardedAt != nil {
			return ErrNotFound
		}
		live, draft := LiveAndDraft(a)
		upd := ts.Automation().UpdateOneID(id).SetStatus(automation.StatusActive)
		switch {
		case draft != nil:
			if err := validateTrigger(draft.TriggerEvent); err != nil {
				return err
			}
			if err := checkIntegration(ctx, ts, draft.IntegrationID); err != nil {
				return err
			}
			upd.SetLiveVersionID(draft.ID).ClearDraftVersionID()
		case live == nil:
			return ErrNothingToPublish
		case a.Status == automation.StatusActive:
			return nil // live and active already
		}
		return upd.Exec(ctx)
	})
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Revert drops the draft: the live version is again the Automation's latest. It is refused
// unless there is a draft above a live version, since a never-published Automation's draft is
// its whole definition. Nothing stands on a draft, so the version goes. Revert is authoring.
func (m *Module) Revert(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		a, err := m.lock(ctx, ts, id)
		if err != nil {
			return err
		}
		live, draft := LiveAndDraft(a)
		if live == nil || draft == nil {
			return ErrNothingToRevert
		}
		// The pointer is cleared through the client first, so the Audit log records the Revert;
		// the key's SET NULL would leave no trace.
		if err := ts.Automation().UpdateOneID(id).ClearDraftVersionID().Exec(ctx); err != nil {
			return err
		}
		return ts.AutomationVersion().DeleteOneID(draft.ID).Exec(ctx)
	})
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Deactivate makes the Automation inactive: it stops new Enrollments, and in-flight ones finish.
// Its live version and draft stay. It is authoring: it can only stop sending, never start it. An
// archived Automation enrolls nobody, so it is as good as absent here: ErrNotFound.
func (m *Module) Deactivate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	n, err := s.Automation().Update().Where(automation.ID(id), automation.DiscardedAtIsNil()).SetStatus(automation.StatusInactive).Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return m.Get(ctx, s, id)
}

func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// refuseWhileEnrolledByRules refuses the removal of an Automation a kept Rule enrolls Contacts
// in, by its live version or its draft (ADR 0049, ADR 0054, ADR 0065): the *inuse.Error names
// the Rules.
func refuseWhileEnrolledByRules(ctx context.Context, s *ent.Scoped, id int64) error {
	using, err := rules.EnrollingInto(ctx, s, id)
	if err != nil || len(using) == 0 {
		return err
	}
	return &inuse.Error{What: inuse.SubjectAutomation, Users: inuse.Rules(using)}
}
