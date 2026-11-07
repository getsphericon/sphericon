// Package automations is the one place an Automation's definition is stored,
// validated and its lifecycle changed (ADR 0016). /site and /api handlers stay thin
// adapters over it. internal/domain/orchestration/automationstep owns the stored Step format and its reader,
// shared by authoring, live-use checks and execution. The definition (Trigger, exit and goal events and
// steps, each send step with its own Integration, ADR 0081) lives in versions (ADR 0049): an edit
// writes the draft, Publish makes it live (ADR 0065).
package automations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/content/versioneddefinition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automationstep"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrNotFound: no such Automation in this Workspace.
	ErrNotFound = validation.NotFound("automations: automation not found", i18n.ErrorsAutomationNotFound)
	// ErrArchived: the Automation is archived, and an archived Automation is read-only until it is
	// unarchived (ADR 0100): no edit, Publish, Revert or Deactivate.
	ErrArchived = validation.Conflict("automations: automation is archived", i18n.ErrorsAutomationArchived)
	// ErrEnrollmentNotFound: no such Enrollment of the Automation in this Workspace.
	ErrEnrollmentNotFound = validation.NotFound("automations: enrollment not found", i18n.ErrorsEnrollmentNotFound)
	// ErrInvalidStep: a step has an unknown type, a negative wait, or a tag step
	// without a tag name, an email step whose content cannot render, or an SMS step whose text
	// takes more segments than the provider sends.
	ErrInvalidStep = validation.Refuse("automations: invalid step", i18n.ErrorsAutomationStepInvalid)
	// ErrInvalidTrigger: the trigger is an infrastructure Event (a send or a delivery notice)
	// that no Automation may start from (ADR 0053).
	ErrInvalidTrigger = validation.Refuse("automations: invalid trigger", i18n.ErrorsAutomationTriggerInvalid)
	// ErrInvalidExitEvent: an exit event is empty, an infrastructure Event, or the trigger itself.
	ErrInvalidExitEvent = validation.Refuse("automations: invalid exit event", i18n.ErrorsAutomationExitEventInvalid)
	// ErrInvalidGoalEvent: the goal event is empty, an infrastructure Event, the trigger itself or
	// one of the exit events.
	ErrInvalidGoalEvent = validation.Refuse("automations: invalid goal event", i18n.ErrorsAutomationGoalEventInvalid)
	// ErrIntegrationNotFound: the Integration does not exist in this Workspace or is archived.
	ErrIntegrationNotFound = validation.Refuse("automations: integration not found", i18n.ErrorsAutomationIntegrationNotFound)

	// ErrNothingToPublish: the Automation has neither a draft nor a live version.
	ErrNothingToPublish = validation.Refuse("automations: nothing to publish", i18n.ErrorsAutomationNothingToPublish)
	// ErrNothingToRevert: the Automation has no draft above a live version. A never-published
	// Automation's draft is its whole definition, so it is not reverted either.
	ErrNothingToRevert = validation.Refuse("automations: nothing to revert", i18n.ErrorsAutomationNothingToRevert)

	// ErrNoVersion: the Automation has no live version: it was never published, so nobody can
	// be enrolled in it.
	ErrNoVersion = errors.New("automations: automation has no live version")
	// ErrInactive: the Automation a Rule enrolls into is inactive or archived by the time the
	// Enrollment is made, so it enrolls nobody (EnrollOnRule).
	ErrInactive = errors.New("automations: automation is not active")

	errBodyAndDoc = errors.New("automations: a step writes a body or a doc, not both")
)

// Encode serializes steps into the stored definition, dropping the fields that do
// not belong to each step's type, trimming tag names and sanitizing an email step's body.
func Encode(steps []automationstep.Step) (string, error) {
	stored := make([]automationstep.Step, 0, len(steps))
	for i := range steps {
		s := &steps[i]
		switch s.Type {
		case automationstep.Wait:
			stored = append(stored, automationstep.Step{Type: automationstep.Wait, Seconds: s.Seconds})
		case automationstep.ApplyTag, automationstep.RemoveTag:
			stored = append(stored, automationstep.Step{Type: s.Type, Tag: strings.TrimSpace(s.Tag)})
		case automationstep.HTTP:
			stored = append(stored, automationstep.Step{Type: automationstep.HTTP, ID: s.ID, URL: s.URL, Headers: s.Headers, Body: s.Body, Secret: s.Secret})
		case automationstep.Telegram:
			stored = append(stored, automationstep.Step{
				Type: automationstep.Telegram, IntegrationID: s.IntegrationID, Text: s.Text,
				ImageURL: strings.TrimSpace(s.ImageURL), Buttons: s.Buttons, LinkPreview: s.LinkPreview,
				UnsubscribeButton: strings.TrimSpace(s.UnsubscribeButton),
			})
		case automationstep.SMS:
			sms := automationstep.Step{Type: automationstep.SMS, IntegrationID: s.IntegrationID, Text: s.Text}
			if !s.TracksLinks() {
				sms.LinkTracking = new(false) // on is the default, stored as nothing
			}
			stored = append(stored, sms)
		default:
			email := automationstep.Step{Type: automationstep.Email, IntegrationID: s.IntegrationID, Subject: s.Subject, Body: emailrender.SanitizeHTML(s.Body)}
			if len(s.Doc) > 0 {
				email.Doc, email.EditorVersion = s.Doc, s.EditorVersion
			}
			stored = append(stored, email)
		}
	}
	b, err := json.Marshal(stored)
	return string(b), err
}

// validate checks each step on its own. linkLength is the length of the Workspace's short tracked
// links, what an SMS step's links count as.
func validate(steps []automationstep.Step, linkLength int) error {
	for i := range steps {
		s := &steps[i]
		switch s.Type {
		case automationstep.Email:
			if err := emailrender.Lint(s.Subject, s.Body, nil).Error; err != nil {
				return ErrInvalidStep.With(err.Error())
			}
		case automationstep.Telegram:
			if strings.TrimSpace(s.Text) == "" {
				return ErrInvalidStep.With("a Telegram step needs a text")
			}
			if err := telegramrender.Check(s.Text); err != nil {
				return ErrInvalidStep.With(err.Error())
			}
			if err := telegramrender.CheckUnsubscribeButton(s.UnsubscribeButton); err != nil {
				return ErrInvalidStep.With(err.Error())
			}
			if err := telegramrender.CheckImage(s.ImageURL); err != nil {
				return ErrInvalidStep.With(err.Error())
			}
			if err := telegramrender.CheckButtons(s.Buttons); err != nil {
				return ErrInvalidStep.With(err.Error())
			}
		case automationstep.SMS:
			if strings.TrimSpace(s.Text) == "" {
				return ErrInvalidStep.With("an SMS step needs a text")
			}
			if err := smsrender.Check(s.Text); err != nil {
				return ErrInvalidStep.With(err.Error())
			}
			// Twilio is the one SMS provider, so its length limit is the one a text is held to. Its
			// links count as the short links they go out as, unless the step keeps them.
			if err := smsrender.CheckLength(s.Text, twilio.MaxLength, lo.Ternary(s.TracksLinks(), linkLength, 0)); err != nil {
				return ErrInvalidStep.With(fmt.Sprintf("step %d: %s", i+1, err))
			}
		case automationstep.HTTP:
		case automationstep.Wait:
			if s.Seconds < 0 {
				return ErrInvalidStep
			}
		case automationstep.ApplyTag, automationstep.RemoveTag:
			if strings.TrimSpace(s.Tag) == "" {
				return ErrInvalidStep
			}
		default:
			return ErrInvalidStep
		}
	}
	return nil
}

// SendStep is a send step of a version, where it stands and what it sends through.
type SendStep struct {
	// Index is the step's position in the version, 0 for the first.
	Index int
	Step  automationstep.Step
}

// Channel is the Channel the step sends on.
func (s SendStep) Channel() channels.Channel {
	ch, _ := s.Step.Type.Channel()
	return ch
}

// Content is the step's Message content, its Channel's own shape (ADR 0078). A new send step kind
// adds its case here beside Kind.Channel.
func (s SendStep) Content() (outboundmessages.Content, error) {
	switch s.Step.Type {
	case automationstep.Email:
		return outboundmessages.EmailContent{Subject: s.Step.Subject, Body: s.Step.Body}, nil
	case automationstep.Telegram:
		return outboundmessages.TelegramContent{
			Text: s.Step.Text, ImageURL: s.Step.ImageURL, Buttons: s.Step.Buttons,
			NoLinkPreview:     s.Step.LinkPreview != nil && !*s.Step.LinkPreview,
			UnsubscribeButton: s.Step.UnsubscribeButton,
		}, nil
	case automationstep.SMS:
		return outboundmessages.SMSContent{Text: s.Step.Text, NoLinkTracking: !s.Step.TracksLinks()}, nil
	default:
		return nil, fmt.Errorf("automations: step type %q has no message content", s.Step.Type)
	}
}

// SendSteps are the send steps of the version, in step order.
func SendSteps(v *ent.AutomationVersion) ([]SendStep, error) {
	steps, err := automationstep.Decode(v.Steps)
	if err != nil {
		return nil, err
	}
	var out []SendStep
	for i := range steps {
		if _, sends := steps[i].Type.Channel(); sends {
			out = append(out, SendStep{Index: i, Step: steps[i]})
		}
	}
	return out, nil
}

// Module is the automations module.
type Module struct {
	cipher   *secrets.Cipher
	renderer *emaildoc.Renderer
	out      *outboundmessages.Module
	life     *versioneddefinition.Lifecycle[ent.Automation, ent.AutomationVersion, edit, definition]
	// now is the Instance clock a report's period ends by and an activation is dated by.
	now func() time.Time
}

// New builds the automations module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017). The cipher seals an HTTP step's secrets; the bus
// opens the transaction of each move of the definition (internal/domain/content/versioneddefinition); the
// renderer builds an email step's body from its document; out is Outbound send, whose Send route
// Publish asks whether the Automation's email steps can send; now is the Instance clock.
func New(cipher *secrets.Cipher, bus *events.Bus, renderer *emaildoc.Renderer, out *outboundmessages.Module, now func() time.Time) *Module {
	m := &Module{cipher: cipher, renderer: renderer, out: out, now: now}
	m.life = versioneddefinition.New(bus, versioned{m: m}, versioneddefinition.Errors{
		NotFound:         ErrNotFound,
		Archived:         ErrArchived,
		NothingToPublish: ErrNothingToPublish,
		NothingToRevert:  ErrNothingToRevert,
	}, now)
	return m
}

// build renders every email step that holds a document to its body, in the same save (ADR 0040,
// the Template rules): a step with a document and a body is refused, and a document that cannot
// be rendered refuses the whole save, naming the step (1-based). A step with no document is
// left as it is, so a body-only write stores no document.
func (m *Module) build(ctx context.Context, steps []automationstep.Step) ([]automationstep.Step, error) {
	out := make([]automationstep.Step, len(steps))
	copy(out, steps)
	for i := range out {
		s := out[i]
		if s.Type != automationstep.Email || len(s.Doc) == 0 {
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

// telegramTriggers are the `telegram.` Events an Automation may start from or end on: what happens
// to a Binding and what the person does. `telegram.sent` is infrastructure, like `email.sent`.
var telegramTriggers = []string{
	events.NameTelegramBound, events.NameTelegramUnbound, "telegram.received", "telegram.clicked", "telegram.unsubscribed",
}

// smsTriggers are the `sms.` Events an Automation may start from or end on: a text the person sent,
// the consent they gave, a link they clicked, and whether a text reached their phone (a delivery
// report, which on SMS says the number works or not, unlike email's infrastructure notices). The
// acceptance (`sms.sent`) is infrastructure, like `email.sent`.
var smsTriggers = []string{
	events.NameSmsReceived, events.NameSmsConfirmed, events.NameSmsClicked, events.NameSmsDelivered, events.NameSmsUndelivered,
}

// validateTrigger refuses an empty trigger and an infrastructure Event as the trigger.
func validateTrigger(trigger string) error {
	if problem := eventProblem(trigger); problem != "" {
		return ErrInvalidTrigger.With(problem)
	}
	return nil
}

// eventProblem says why the action can neither start nor end an Automation: it is empty, or an
// infrastructure Event (ADR 0053). Empty when it can.
func eventProblem(action string) string {
	t := strings.ToLower(strings.TrimSpace(action))
	if t == "" {
		return "the event is empty"
	}
	if strings.HasPrefix(t, "email.") && !slices.Contains(engagementTriggers, t) {
		return fmt.Sprintf("%q is not a customer action; use %s", action, strings.Join(engagementTriggers, ", "))
	}
	if strings.HasPrefix(t, "telegram.") && !slices.Contains(telegramTriggers, t) {
		return fmt.Sprintf("%q is not a customer action; use %s", action, strings.Join(telegramTriggers, ", "))
	}
	if strings.HasPrefix(t, "sms.") && !slices.Contains(smsTriggers, t) {
		return fmt.Sprintf("%q is not a customer action; use %s", action, strings.Join(smsTriggers, ", "))
	}
	return ""
}

// validateEvents checks the definition's trigger, each exit event and the goal event by the
// trigger's rule (validateTrigger), and refuses an exit or goal event equal to the trigger and a
// goal that is also an exit event: one Event would then both enroll and end, or end two ways.
func validateEvents(def definition) error {
	if err := validateTrigger(def.trigger); err != nil {
		return err
	}
	trigger := strings.TrimSpace(def.trigger)
	for _, e := range def.exits {
		if problem := eventProblem(e); problem != "" {
			return ErrInvalidExitEvent.With(problem)
		}
		if e == trigger {
			return ErrInvalidExitEvent.With(fmt.Sprintf("%q is the trigger", e))
		}
	}
	if def.goal == nil {
		return nil
	}
	goal := *def.goal
	if problem := eventProblem(goal); problem != "" {
		return ErrInvalidGoalEvent.With(problem)
	}
	if goal == trigger {
		return ErrInvalidGoalEvent.With(fmt.Sprintf("%q is the trigger", goal))
	}
	if slices.Contains(def.exits, goal) {
		return ErrInvalidGoalEvent.With(fmt.Sprintf("%q is also an exit event", goal))
	}
	return nil
}

// normalizeExits turns a list of exit events into its stored form: each trimmed, the first of a
// repeat kept, never nil.
func normalizeExits(actions []string) []string {
	out := make([]string, 0, len(actions))
	for _, e := range actions {
		e = strings.TrimSpace(e)
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out
}

// normalizeGoal turns a goal event into its stored form: trimmed, nil for none.
func normalizeGoal(goal *string) *string {
	if goal == nil {
		return nil
	}
	g := strings.TrimSpace(*goal)
	return &g
}

// CreateInput is the data for a new Automation. It is always created inactive: publishing is a
// separate, send-class operation (ADR 0065).
type CreateInput struct {
	Name         string
	TriggerEvent string
	// ExitEvents end an active Enrollment early and GoalEvent ends it as having reached its goal;
	// nil is none.
	ExitEvents []string
	GoalEvent  *string
	Steps      []automationstep.Step
}

// UpdateInput changes an Automation; nil fields are left alone. The name is the Automation's own;
// the rest is the definition, written to the draft (ADR 0065).
type UpdateInput struct {
	Name         *string
	TriggerEvent *string
	// ExitEvents replaces the exit events; nil leaves them alone.
	ExitEvents *[]string
	// GoalEvent sets the goal event, and a clear removes it.
	GoalEvent patch.Field[string]
	Steps     *[]automationstep.Step
}

// LiveAndDraft returns the Automation's live version and its draft, each nil when it has none.
// The Automations this module returns carry both.
func LiveAndDraft(a *ent.Automation) (live, draft *ent.AutomationVersion) {
	return versioneddefinition.LiveAndDraft(a)
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
	steps, err := automationstep.Decode(v.Steps)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(steps, func(st automationstep.Step) bool {
		return (st.Type == automationstep.ApplyTag || st.Type == automationstep.RemoveTag) && strings.TrimSpace(st.Tag) == name
	}), nil
}

// Get returns one Automation of the Workspace, archived or not: an archived Automation is
// still addressable (viewed, unarchived, its Enrollments listed).
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	a, err := withVersions(s.Automation().QueryWithArchived().Where(automation.ID(id))).Only(ctx)
	return a, notFound(err)
}

// Create stores a new inactive Automation whose first version is its draft: nothing is live until
// it is published (ADR 0065).
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Automation, error) {
	steps, err := m.build(ctx, in.Steps)
	if err != nil {
		return nil, err
	}
	owner := func(ctx context.Context, ts *ent.Scoped) (int64, error) {
		a, err := ts.Automation().Create().SetName(in.Name).Save(ctx)
		if err != nil {
			return 0, err
		}
		return a.ID, nil
	}
	e := edit{trigger: &in.TriggerEvent, exits: &in.ExitEvents, goal: patch.FromPtr(in.GoalEvent)}
	if in.Steps != nil {
		e.steps = &steps
	}
	return m.life.Create(ctx, s, owner, e)
}

// Update renames the Automation and writes its definition to the draft (ADR 0065,
// internal/domain/content/versioneddefinition): the fields left out keep the draft's, or the live version's when
// there is no draft. The live version is never rewritten: the first edit above it creates the
// draft, the next ones rewrite the draft in place. A definition equal to the draft's (or, with no
// draft, the live version's) stores nothing. An edit of the Trigger, exit or goal events checks the
// three together (validateEvents), so a new Trigger equal to a stored exit or goal event is refused
// too. The status is untouched. An archived Automation is read-only: ErrArchived.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Automation, error) {
	e := edit{trigger: in.TriggerEvent, exits: in.ExitEvents, goal: in.GoalEvent}
	if in.Steps != nil {
		built, err := m.build(ctx, *in.Steps)
		if err != nil {
			return nil, err
		}
		e.steps = &built
	}
	rename := func(ctx context.Context, ts *ent.Scoped, a *ent.Automation) error {
		if in.Name == nil || *in.Name == a.Name {
			return nil
		}
		return ts.Automation().UpdateOneID(a.ID).SetName(*in.Name).Exec(ctx)
	}
	return m.life.Edit(ctx, s, id, rename, e)
}

// edit is a change of an Automation's definition; nil fields keep the base's. The steps are built
// (their documents rendered) before the transaction.
type edit struct {
	trigger *string
	exits   *[]string
	goal    patch.Field[string]
	steps   *[]automationstep.Step
}

// definition is the content of an Automation version as stored: its Trigger, the events that end
// an Enrollment early (exits) and the one that ends it as having reached its goal (nil for none),
// and its encoded, sealed steps, each send step naming its Integration.
type definition struct {
	trigger string
	exits   []string
	goal    *string
	steps   string
}

// versioned is the Automation's part of the lifecycle (versioneddefinition.Definition).
type versioned struct{ m *Module }

func (versioned) Store(s *ent.Scoped) ent.VersionedStore[ent.Automation, ent.AutomationVersion] {
	return s.Automation().Pointers()
}

// Merge lays the edit over the base version (nil for a new Automation) and validates what it
// changes: the Trigger with the exit and goal events, checked together (validateEvents), and the
// steps, each send step's Integration kept (not archived). An HTTP step that names one of the
// base's steps by ID keeps the secrets it does not resend.
func (v versioned) Merge(ctx context.Context, ts *ent.Scoped, base *ent.AutomationVersion, e edit) (definition, error) {
	def := definition{exits: []string{}, steps: "[]"}
	var stored []automationstep.Step
	if base != nil {
		var err error
		if stored, err = automationstep.Decode(base.Steps); err != nil {
			return definition{}, err
		}
		if def, err = canonical(base); err != nil {
			return definition{}, err
		}
	}
	if e.trigger != nil {
		def.trigger = *e.trigger
	}
	if e.exits != nil {
		def.exits = normalizeExits(*e.exits)
	}
	def.goal = normalizeGoal(e.goal.Over(def.goal))
	if e.trigger != nil || e.exits != nil || e.goal.Changes() {
		if err := validateEvents(def); err != nil {
			return definition{}, err
		}
	}
	if e.steps != nil {
		linkLength, err := v.m.out.ShortLinkLength(ctx, ts)
		if err != nil {
			return definition{}, err
		}
		if err := validate(*e.steps, linkLength); err != nil {
			return definition{}, err
		}
		for i := range *e.steps {
			if err := checkIntegration(ctx, ts, (*e.steps)[i].IntegrationID); err != nil {
				return definition{}, err
			}
		}
		sealed, err := v.m.sealSteps(*e.steps, stored)
		if err != nil {
			return definition{}, err
		}
		if def.steps, err = Encode(sealed); err != nil {
			return definition{}, err
		}
	}
	return def, nil
}

// canonical is the definition a version holds, its steps in the form Encode writes them, so that
// an unchanged edit compares equal whatever wrote the stored JSON.
func canonical(v *ent.AutomationVersion) (definition, error) {
	stored, err := automationstep.Decode(v.Steps)
	if err != nil {
		return definition{}, err
	}
	steps, err := Encode(stored)
	if err != nil {
		return definition{}, err
	}
	return definition{trigger: v.TriggerEvent, exits: normalizeExits(v.ExitEvents), goal: v.GoalEvent, steps: steps}, nil
}

func (versioned) Equal(v *ent.AutomationVersion, def definition) bool {
	held, err := canonical(v)
	return err == nil && held.trigger == def.trigger && slices.Equal(held.exits, def.exits) && samePtr(held.goal, def.goal) &&
		held.steps == def.steps
}

func (versioned) Write(ctx context.Context, ts *ent.Scoped, id int64, number int, draft *ent.AutomationVersion, def definition) (int64, error) {
	if draft != nil {
		upd := ts.AutomationVersion().UpdateOneID(draft.ID).
			SetTriggerEvent(def.trigger).
			SetExitEvents(def.exits).
			SetNillableGoalEvent(def.goal).
			SetSteps(def.steps)
		if def.goal == nil {
			upd.ClearGoalEvent()
		}
		return draft.ID, upd.Exec(ctx)
	}
	created, err := ts.AutomationVersion().Create().
		SetAutomationID(id).
		SetVersion(number).
		SetTriggerEvent(def.trigger).
		SetExitEvents(def.exits).
		SetNillableGoalEvent(def.goal).
		SetSteps(def.steps).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

// Publishable checks the Trigger, exit and goal events of the version going live again, and the
// Integration of each of its send steps must still be kept. Publish arms every send step (ADR
// 0081), so the version is refused with outboundmessages.ErrRouteHeld while the Send route of any of them,
// on the step's Channel through the Integration it names (or that Channel's default), cannot send:
// an edit may name such an Integration, only Publish is refused, and a route that stops sending
// after it holds the sends at that step instead. A Telegram step whose text, as written, is over
// Telegram's limit is refused too.
func (v versioned) Publishable(ctx context.Context, ts *ent.Scoped, version *ent.AutomationVersion) error {
	if err := validateEvents(definition{trigger: version.TriggerEvent, exits: version.ExitEvents, goal: version.GoalEvent}); err != nil {
		return err
	}
	sends, err := SendSteps(version)
	if err != nil {
		return err
	}
	for i := range sends {
		st := &sends[i]
		if st.Step.Type != automationstep.Telegram {
			continue
		}
		if err := telegramrender.CheckWritten(st.Step.Text, st.Step.ImageURL != ""); err != nil {
			return ErrInvalidStep.With(fmt.Sprintf("step %d: %s", st.Index+1, err))
		}
	}
	type routeKey struct {
		ch          channels.Channel
		integration int64
	}
	asked := map[routeKey]bool{}
	for i := range sends {
		st := &sends[i]
		key := routeKey{st.Channel(), lo.FromPtr(st.Step.IntegrationID)}
		if asked[key] {
			continue
		}
		asked[key] = true
		if err := checkIntegration(ctx, ts, st.Step.IntegrationID); err != nil {
			return err
		}
		route, err := v.m.out.Route(ctx, ts, st.Channel(), st.Step.IntegrationID)
		if err != nil {
			return err
		}
		if err := route.Armable(); err != nil {
			return err
		}
	}
	return nil
}

// Senders returns the Workspace's active, kept Automations whose live version has a send step,
// each with its live version loaded, in id order: the ones that send through a Send route whenever
// a run reaches such a step. A draft sends nothing until it is published.
func Senders(ctx context.Context, s *ent.Scoped) ([]*ent.Automation, error) {
	active, err := s.Automation().Query().
		Where(automation.StatusEQ(automation.StatusActive), automation.HasLiveVersion()).
		WithLiveVersion().
		Order(ent.Asc(automation.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	senders := active[:0]
	for _, a := range active {
		sends, err := SendSteps(a.Edges.LiveVersion)
		if err != nil {
			return nil, err
		}
		if len(sends) > 0 {
			senders = append(senders, a)
		}
	}
	return senders, nil
}

// Editable never refuses: the lifecycle itself refuses an archived Automation (ErrArchived), and
// an Automation is never discarded.
func (versioned) Editable(*ent.Automation) error { return nil }

// Archivable refuses to archive an Automation while it is Live use (ADR 0099): an active Rule
// enrolls Contacts in it through its live version, or it has active Enrollments. The *liveuse.Error
// names them. A Rule that is not active does not hold it: its Publish refuses an archived
// Automation.
func (versioned) Archivable(ctx context.Context, ts *ent.Scoped, a *ent.Automation) error {
	enrolling, err := rules.ActiveEnrollingInto(ctx, ts, a.ID)
	if err != nil {
		return err
	}
	if len(enrolling) > 0 {
		return &liveuse.Error{What: liveuse.SubjectAutomation, Users: liveuse.Rules(enrolling)}
	}
	active, err := ts.AutomationRun().Query().
		Where(automationrun.AutomationID(a.ID), automationrun.StatusEQ(automationrun.StatusActive)).
		Count(ctx)
	if err != nil {
		return err
	}
	if active > 0 {
		return &liveuse.Error{What: liveuse.SubjectAutomation, Users: []liveuse.User{liveuse.Enrollments(a.ID, a.Name, active)}}
	}
	return nil
}

// samePtr reports whether two optional values are both absent or both present and equal.
func samePtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// checkIntegration refuses an Integration that is missing or archived, the rule a Broadcast
// save applies too (messaging.IntegrationKept). A live version whose step already points at one
// keeps it as history.
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

// Archive puts an Automation aside (ADR 0100, internal/domain/content/versioneddefinition): it leaves the lists
// and enrolls nobody, and its Enrollments stay. An Automation is archived, never deleted, a
// never-published one too. Archive also makes the Automation inactive, so Unarchive never re-arms
// it without a Publish. An Automation with active Enrollments, or one an active Rule enrolls
// Contacts in, is refused with a *liveuse.Error naming them. Archiving an archived Automation
// changes nothing. It returns the Automation.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.life.Archive(ctx, s, id)
}

// Unarchive brings an archived Automation back to the lists, inactive: only Publish makes it
// enroll again. Unarchiving one that is not archived changes nothing. It returns the Automation.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.life.Unarchive(ctx, s, id)
}

// Publish makes the draft live and the Automation active (internal/domain/content/versioneddefinition): new
// Enrollments begin on the draft's definition, and the old live version stays as history; the
// Enrollments already on it finish there. With no draft it makes an inactive live version active
// again and leaves an active one as it is; with neither it is refused. The Trigger, exit and goal
// events of the version going live are checked again, and the Integration of each send step must
// still be kept and its Send route able to send (outboundmessages.ErrRouteHeld). Publish is send-class (ADR
// 0065): the surfaces gate it by the publish scope. An archived Automation is unarchived before it
// is published: ErrArchived.
func (m *Module) Publish(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.life.Publish(ctx, s, id)
}

// Revert drops the draft: the live version is again the Automation's latest. It is refused
// unless there is a draft above a live version, since a never-published Automation's draft is
// its whole definition. Revert is authoring. An archived Automation is read-only: ErrArchived.
func (m *Module) Revert(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.life.Revert(ctx, s, id)
}

// Deactivate makes the Automation inactive: it stops new Enrollments, and in-flight ones finish.
// Its live version and draft stay. It is authoring: it can only stop sending, never start it. An
// archived Automation is read-only: ErrArchived.
func (m *Module) Deactivate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	return m.life.Deactivate(ctx, s, id)
}

func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}
