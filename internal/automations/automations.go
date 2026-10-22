// Package automations is the one place an Automation's steps are stored,
// validated and its lifecycle changed (ADR 0016). /site and /api handlers stay thin
// adapters over it; the step executor (internal/jobs) decodes the same Step format,
// so the storage JSON has a single definition. The steps live in versions (ADR 0049).
package automations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/secrets"
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
	ErrNotFound = errors.New("automations: automation not found")
	// ErrInvalidStep: a step has an unknown type, a negative wait, or a tag step
	// without a tag name, or an email step whose content cannot render.
	ErrInvalidStep = errors.New("automations: invalid step")
	// ErrInvalidTrigger: the trigger is an infrastructure Event (a send or a delivery notice)
	// that no Automation may start from (ADR 0053).
	ErrInvalidTrigger = errors.New("automations: invalid trigger")

	// ErrNoVersion: the Automation has no version yet. Only an Automation made before versions
	// existed can be in that state, until the backfill job (internal/jobs) has run.
	ErrNoVersion = errors.New("automations: automation has no version yet")

	errBodyAndDoc = errors.New("automations: a step writes a body or a doc, not both")
)

// build renders every email step that holds a document to its body, in the same save (ADR 0040,
// the Template rules): a step with a document and a body is refused, and a document that cannot
// be rendered refuses the whole save, naming the step (1-based). A step with no document is
// left as it is, so a body-only write stores no document.
func build(ctx context.Context, steps []Step) ([]Step, error) {
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
		c, err := emailrender.ResolveContent(ctx, &s.Subject, body, s.Doc, errBodyAndDoc)
		switch {
		case errors.Is(err, errBodyAndDoc):
			return nil, fmt.Errorf("%w: step %d: write a body or a doc, not both", ErrInvalidStep, i+1)
		case errors.Is(err, emailrender.ErrInvalidContent):
			return nil, fmt.Errorf("%w: step %d: %w", ErrInvalidStep, i+1, err)
		case err != nil:
			return nil, err
		}
		out[i].Body, out[i].EditorVersion = c.Authored.Body, c.Authored.EditorVersion
	}
	return out, nil
}

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
				return fmt.Errorf("%w: %w", ErrInvalidStep, err)
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
	cipher *secrets.Cipher
	bus    *events.Bus
}

// New builds the automations module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017). The cipher seals an HTTP step's secrets; the bus
// opens the transaction that saves an Automation together with its steps.
func New(cipher *secrets.Cipher, bus *events.Bus) *Module { return &Module{cipher: cipher, bus: bus} }

// IsInvalid reports whether err refuses the Automation's input (a step or the trigger), which an
// API surface renders as 422.
func IsInvalid(err error) bool {
	return errors.Is(err, ErrInvalidStep) || errors.Is(err, ErrInvalidTrigger)
}

// engagementTriggers are the `email.` Events an Automation may start from: what a recipient does.
// The others (sent, delivered, delayed, dropped, bounced, complained) are infrastructure, kept
// for the figures and the log (ADR 0053).
var engagementTriggers = []string{events.NameEmailOpened, events.NameEmailClicked, events.NameEmailUnsubscribed}

// validateTrigger refuses an infrastructure Event as the trigger.
func validateTrigger(trigger string) error {
	t := strings.ToLower(strings.TrimSpace(trigger))
	if strings.HasPrefix(t, "email.") && !slices.Contains(engagementTriggers, t) {
		return fmt.Errorf("%w: %q is not a trigger; use %s", ErrInvalidTrigger, trigger, strings.Join(engagementTriggers, ", "))
	}
	return nil
}

// CreateInput is the data for a new Automation. It is always created as a draft:
// activating is a separate, deliberate operation.
type CreateInput struct {
	Name         string
	TriggerEvent string
	Steps        []Step
}

// UpdateInput changes an Automation; nil fields are left alone.
type UpdateInput struct {
	Name         *string
	TriggerEvent *string
	Steps        *[]Step
}

// latest narrows the versions an Automation query loads to the newest one of each Automation:
// the Automation's current steps (CurrentSteps reads them).
func latest(q *ent.AutomationVersionQuery) {
	q.Where(func(s *sql.Selector) {
		newest := sql.Table(automationversion.Table).As("newest")
		s.Where(sql.EQ(s.C(automationversion.FieldVersion),
			sql.Select(sql.Max(newest.C(automationversion.FieldVersion))).From(newest).
				Where(sql.ColumnsEQ(newest.C(automationversion.FieldAutomationID), s.C(automationversion.FieldAutomationID)))))
	})
}

// CurrentSteps decodes the steps of the Automation's latest version, which the Automations this
// module returns carry. An Automation that was loaded some other way has none.
func CurrentSteps(a *ent.Automation) ([]Step, error) {
	if len(a.Edges.Versions) == 0 {
		return nil, nil
	}
	return Decode(a.Edges.Versions[0].Steps)
}

// Loaded returns the Workspace's Automations matching preds, each carrying its current steps
// (CurrentSteps). Archived Automations are not loaded: they enroll nobody and use nothing. A
// live-dependency check reads them to see which Tag, Integration or Sending domain an
// Automation's steps use (ADR 0049).
func Loaded(ctx context.Context, s *ent.Scoped, preds ...predicate.Automation) ([]*ent.Automation, error) {
	return s.Automation().Query().Where(preds...).WithVersions(latest).Order(ent.Asc(automation.FieldID)).All(ctx)
}

// UsingTag returns the Workspace's Automations, archived ones included, whose current steps
// apply or remove the Tag by its name, in id order. An archived Automation is restorable, so it
// still names the Tag (ADR 0049).
func UsingTag(ctx context.Context, s *ent.Scoped, name string) ([]*ent.Automation, error) {
	all, err := s.Automation().QueryWithDiscarded().WithVersions(latest).Order(ent.Asc(automation.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var using []*ent.Automation
	for _, a := range all {
		steps, err := CurrentSteps(a)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(steps, func(st Step) bool {
			return (st.Type == StepApplyTag || st.Type == StepRemoveTag) && strings.TrimSpace(st.Tag) == name
		}) {
			using = append(using, a)
		}
	}
	return using, nil
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
			return rows().WithVersions(latest).Order(ent.Desc(automation.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Get returns one Automation of the Workspace, archived or not: an archived Automation is
// still addressable (viewed, restored, its Enrollments listed).
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	a, err := s.Automation().QueryWithDiscarded().Where(automation.ID(id)).WithVersions(latest).Only(ctx)
	return a, notFound(err)
}

// Create stores a new draft Automation with its first version.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Automation, error) {
	if err := validateTrigger(in.TriggerEvent); err != nil {
		return nil, err
	}
	built, err := build(ctx, in.Steps)
	if err != nil {
		return nil, err
	}
	in.Steps = built
	if err := validate(in.Steps); err != nil {
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
			SetTriggerEvent(in.TriggerEvent).
			SetStatus(automation.StatusDraft).
			Save(ctx)
		if err != nil {
			return err
		}
		id = a.ID
		_, err = ts.AutomationVersion().Create().SetAutomationID(a.ID).SetVersion(1).SetSteps(def).Save(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Update changes the Automation's name, trigger or steps. The status is untouched.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Automation, error) {
	if in.TriggerEvent != nil {
		if err := validateTrigger(*in.TriggerEvent); err != nil {
			return nil, err
		}
	}
	var steps []Step
	if in.Steps != nil {
		built, err := build(ctx, *in.Steps)
		if err != nil {
			return nil, err
		}
		if err := validate(built); err != nil {
			return nil, err
		}
		steps = built
	}
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if _, err := ts.Automation().UpdateOneID(id).
			SetNillableName(in.Name).
			SetNillableTriggerEvent(in.TriggerEvent).
			Save(ctx); err != nil {
			return err
		}
		if in.Steps == nil {
			return nil
		}
		return m.saveSteps(ctx, ts, id, steps)
	})
	if err != nil {
		return nil, notFound(err)
	}
	return m.Get(ctx, s, id)
}

// saveSteps stores steps as the Automation's current ones (ADR 0049). The latest version is
// rewritten in place when no Enrollment stands on it, so a draft edited many times stays one
// version; otherwise the steps become a new version and the Enrollments keep the one they began
// on. Saving steps identical to the latest version's stores nothing.
func (m *Module) saveSteps(ctx context.Context, s *ent.Scoped, automationID int64, steps []Step) error {
	// Lock the Automation row for the rest of the transaction: Enroll inserts an Enrollment
	// that references it, which waits on this lock, so no Enrollment can begin between the
	// "none stands on this version" check below and the in-place rewrite, and the check sees
	// every Enrollment that began before the lock was taken.
	if _, err := s.Automation().QueryWithDiscarded().Where(automation.ID(automationID)).ForUpdate().OnlyID(ctx); err != nil {
		return notFound(err)
	}
	cur, err := s.AutomationVersion().Query().
		Where(automationversion.AutomationID(automationID)).
		Order(ent.Desc(automationversion.FieldVersion)).
		First(ctx)
	if ent.IsNotFound(err) {
		return ErrNoVersion
	}
	if err != nil {
		return err
	}
	stored, err := Decode(cur.Steps)
	if err != nil {
		return err
	}
	sealed, err := m.sealSteps(steps, stored)
	if err != nil {
		return err
	}
	def, err := Encode(sealed)
	if err != nil {
		return err
	}
	if def == cur.Steps {
		return nil
	}
	standing, err := s.AutomationRun().Query().Where(automationrun.VersionID(cur.ID)).Exist(ctx)
	if err != nil {
		return err
	}
	if !standing {
		return s.AutomationVersion().UpdateOneID(cur.ID).SetSteps(def).Exec(ctx)
	}
	return s.AutomationVersion().Create().
		SetAutomationID(automationID).
		SetVersion(cur.Version + 1).
		SetSteps(def).
		Exec(ctx)
}

// Delete purges an Automation that has no Enrollments, with its versions (ADR 0049). One with
// any Enrollment is history and is archived instead: an *inuse.Error carrying their count.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	if err := refuseWhileEnrolledByRules(ctx, s, id); err != nil {
		return err
	}
	n, err := s.Automation().Purge().Where(automation.ID(id), automation.Not(automation.HasRuns())).Exec(ctx)
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
	return &inuse.Error{What: inuse.SubjectAutomation, Users: []inuse.User{inuse.Enrollments(id, a.Name, enrollments)}}
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

// Activate starts enrolling Contacts into the Automation.
func (m *Module) Activate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.setStatus(ctx, s, id, automation.StatusActive)
}

// Deactivate stops new enrollments; in-flight Enrollments finish.
func (m *Module) Deactivate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.setStatus(ctx, s, id, automation.StatusDraft)
}

// setStatus changes the status of a kept Automation. An archived one enrolls nobody and is
// restored before it is switched, so it is as good as absent here: ErrNotFound.
func (m *Module) setStatus(ctx context.Context, s *ent.Scoped, id int64, status automation.Status) (*ent.Automation, error) {
	n, err := s.Automation().Update().Where(automation.ID(id), automation.DiscardedAtIsNil()).SetStatus(status).Save(ctx)
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

// refuseWhileEnrolledByRules refuses the removal of an Automation an active Rule enrolls Contacts
// in (ADR 0049, ADR 0054): the *inuse.Error names the Rules.
func refuseWhileEnrolledByRules(ctx context.Context, s *ent.Scoped, id int64) error {
	using, err := rules.EnrollingInto(ctx, s, id)
	if err != nil || len(using) == 0 {
		return err
	}
	return &inuse.Error{What: inuse.SubjectAutomation, Users: inuse.Rules(using)}
}
