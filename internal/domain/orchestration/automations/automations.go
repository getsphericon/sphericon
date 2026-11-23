// Package automations is the one place an Automation's definition is stored,
// validated and its lifecycle changed (ADR 0016). /site and /api handlers stay thin
// adapters over it. internal/kernel/automationstep owns the stored Step format and its reader,
// shared by authoring, live-use checks and execution. The definition (Trigger, exit and goal events and
// steps, each send step with its own Integration, ADR 0081) lives in versions (ADR 0049): an edit
// writes the draft, Publish makes it live (ADR 0065). StepExecution walks the Enrollments through
// their steps (ADR 0112).
package automations

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/content/versioneddefinition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/jsonvalue"
	"github.com/getsphericon/sphericon/internal/kernel/optional"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/smsrender"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
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
	// ErrRevisionChanged is an edit, Publish or Revert made on a revision the Automation has moved past
	// (ADR 0119): someone changed it since the editor read it. Nothing is written.
	ErrRevisionChanged = validation.RevisionChanged("automations: automation changed since it was read", i18n.ErrorsAutomationRevisionChanged)

	// ErrNoVersion: the Automation has no live version: it was never published, so nobody can
	// be enrolled in it.
	ErrNoVersion = validation.Conflict("automations: automation has no live version", i18n.ErrorsAutomationNoVersion)
	// ErrInactive: the Automation a Rule enrolls into is inactive or archived by the time the
	// Enrollment is made, so it enrolls nobody (StepExecution.EnrollOnRule).
	ErrInactive = validation.Conflict("automations: automation is not active", i18n.ErrorsAutomationInactive)

	errBodyAndDoc = errors.New("automations: a step writes a body or a doc, not both")
	// errEventWithoutTime: an Event handed to OnEvent without when it occurred.
	errEventWithoutTime = errors.New("automations: the Event has no occurred_at")
)

// StoredSteps are the steps as a version stores them, each payload in its stored form
// (automationstep.Payload.Stored).
func StoredSteps(p automationstep.Path) automationstep.Path {
	return automationstep.Rewrite(p, func(_ int, st *automationstep.Step) automationstep.Step {
		return automationstep.Step{ID: st.ID, Payload: st.Payload.Stored()}
	})
}

// refuse is ErrInvalidStep naming the Step by its id and kind (ADR 0122), and why.
func refuse(st *automationstep.Step, why string) error {
	return ErrInvalidStep.With(fmt.Sprintf("step %s (%s): %s", st.ID, st.Kind(), why))
}

// validate checks each step on its own; the steps carry their ids (identify). linkLength is the
// length of the Workspace's short tracked links, what an SMS step's links count as.
func validate(steps automationstep.Path, linkLength int) error {
	for _, st := range automationstep.All(steps) {
		switch s := st.Payload.(type) {
		case automationstep.Email:
			if err := emailrender.Lint(s.Subject, s.Body, nil).Error; err != nil {
				return refuse(st, err.Error())
			}
		case automationstep.Telegram:
			if strings.TrimSpace(s.Text) == "" {
				return refuse(st, "a Telegram step needs a text")
			}
			for _, err := range []error{
				telegramrender.Check(s.Text),
				telegramrender.CheckUnsubscribeButton(s.UnsubscribeButton),
				telegramrender.CheckImage(s.ImageURL),
				telegramrender.CheckButtons(s.Buttons),
			} {
				if err != nil {
					return refuse(st, err.Error())
				}
			}
		case automationstep.SMS:
			if strings.TrimSpace(s.Text) == "" {
				return refuse(st, "an SMS step needs a text")
			}
			if err := smsrender.Check(s.Text); err != nil {
				return refuse(st, err.Error())
			}
			// Twilio is the one SMS provider, so its length limit is the one a text is held to. Its
			// links count as the short links they go out as, unless the step keeps them.
			if err := smsrender.CheckLength(s.Text, twilio.MaxLength, lo.Ternary(s.TracksLinks(), linkLength, 0)); err != nil {
				return refuse(st, err.Error())
			}
		case automationstep.HTTP:
		case automationstep.Wait:
			if s.Seconds < 0 {
				return refuse(st, "a wait cannot be negative")
			}
		case automationstep.Tagging:
			if strings.TrimSpace(s.TagName()) == "" {
				return refuse(st, "a tag step needs a tag name")
			}
		case automationstep.End:
		default:
			return refuse(st, "the step has no known kind")
		}
	}
	return nil
}

// identify gives the path and each step without an id one, keeping the ids sent; a malformed id
// or one used twice in the version refuses the save, naming the Step or the path (ADR 0122).
func identify(steps automationstep.Path) (automationstep.Path, error) {
	out, err := automationstep.Identify(steps, automationstep.NewID)
	if bad, ok := errors.AsType[*automationstep.BadIDError](err); ok {
		return automationstep.Path{}, ErrInvalidStep.With(bad.Detail)
	}
	return out, err
}

// checkEnds refuses a Step that follows an End in its path, naming it by its id and kind: an End
// finishes its path (ADR 0124), so the Step would never run. The steps carry their ids (identify).
func checkEnds(steps automationstep.Path) error {
	if st, after := automationstep.AfterEnd(steps); after {
		return refuse(st, "it comes after an end step, which finishes its path")
	}
	return nil
}

// SendStep is a send step of a version: the Step, named by its id, and what it sends through.
type SendStep struct {
	Step automationstep.Step
}

// Channel is the Channel the step sends on.
func (s SendStep) Channel() channels.Channel {
	ch, _ := s.Step.Kind().Channel()
	return ch
}

// Content is the step's Message content, its Channel's own shape (ADR 0078). A new send step kind
// adds its case here beside Kind.Channel.
func (s SendStep) Content() (outboundmessages.Content, error) {
	switch p := s.Step.Payload.(type) {
	case automationstep.Email:
		return outboundmessages.EmailContent{Subject: p.Subject, Body: p.Body}, nil
	case automationstep.Telegram:
		return outboundmessages.TelegramContent{
			Text: p.Text, ImageURL: p.ImageURL, Buttons: p.Buttons,
			NoLinkPreview:     p.LinkPreview != nil && !*p.LinkPreview,
			UnsubscribeButton: p.UnsubscribeButton,
		}, nil
	case automationstep.SMS:
		return outboundmessages.SMSContent{Text: p.Text, NoLinkTracking: !p.TracksLinks()}, nil
	default:
		return nil, fmt.Errorf("automations: step type %q has no message content", s.Step.Kind())
	}
}

// SendSteps are the send steps of the version, in step order.
func SendSteps(v *ent.AutomationVersion) []SendStep {
	var out []SendStep
	for _, st := range automationstep.All(v.Steps) {
		if _, sends := st.Kind().Channel(); sends {
			out = append(out, SendStep{Step: *st})
		}
	}
	return out
}

// Module is the automations module.
type Module struct {
	cipher   *secrets.Cipher
	bus      *events.Bus
	renderer *emaildoc.Renderer
	out      *outboundmessages.Module
	clicks   *linkclick.Module
	rules    *rules.Module
	revenues *orders.Revenues
	life     *versioneddefinition.Lifecycle[ent.Automation, ent.AutomationVersion, edit, definition]
	// now is the Instance clock a report's period ends by, an activation is dated by and an
	// Enrollment's exit is.
	now func() time.Time
}

// New builds the automations module. It holds no client: every call takes the
// Workspace's scoped client (ADR 0017). The cipher seals an HTTP step's secrets; the bus
// opens the transaction of each move of the definition (internal/domain/content/versioneddefinition)
// and records an Enrollment's exit in the transaction Exit is given; the renderer builds an email
// step's body from its document; out is Outbound send, whose Send route Publish asks whether the
// Automation's email steps can send; clicks counts the Link clicks of its report; enrollers reads
// the Rules that enroll Contacts in an Automation, which hold it against its Archive; now is the
// Instance clock.
func New(cipher *secrets.Cipher, bus *events.Bus, renderer *emaildoc.Renderer, out *outboundmessages.Module, clicks *linkclick.Module, enrollers *rules.Module, now func() time.Time) *Module {
	m := &Module{cipher: cipher, bus: bus, renderer: renderer, out: out, clicks: clicks, rules: enrollers, now: now,
		revenues: orders.NewRevenues()}
	m.life = versioneddefinition.New(bus, versioned{m: m}, versioneddefinition.Errors{
		NotFound:         ErrNotFound,
		Archived:         ErrArchived,
		NothingToPublish: ErrNothingToPublish,
		NothingToRevert:  ErrNothingToRevert,
		RevisionChanged:  ErrRevisionChanged,
	}, now)
	return m
}

// build renders every email step that holds a document to its body, in the same save (ADR 0040,
// the Template rules): a step with a document and a body is refused, and a document that cannot
// be rendered refuses the whole save, naming the step by its id and kind. A step with no document
// is left as it is, so a body-only write stores no document. The steps are identified first
// (identify), so every refusal of the save can name its Step.
func (m *Module) build(ctx context.Context, written automationstep.Path) (automationstep.Path, error) {
	steps, err := identify(written)
	if err != nil {
		return automationstep.Path{}, err
	}
	return automationstep.Map(steps, func(_ int, st *automationstep.Step) (automationstep.Step, error) {
		s, ok := st.Payload.(automationstep.Email)
		if !ok || len(s.Doc) == 0 {
			return *st, nil
		}
		var body *string
		if s.Body != "" {
			body = &s.Body
		}
		c, err := emailrender.ResolveContent(ctx, m.renderer, &s.Subject, body, s.Doc, errBodyAndDoc)
		switch {
		case errors.Is(err, errBodyAndDoc):
			return automationstep.Step{}, refuse(st, "write a body or a doc, not both")
		case errors.Is(err, emailrender.ErrInvalidContent):
			return automationstep.Step{}, refuse(st, err.Error())
		case err != nil:
			return automationstep.Step{}, err
		}
		s.Body, s.EditorVersion = c.Authored.Body, c.Authored.EditorVersion
		return automationstep.Step{ID: st.ID, Payload: s}, nil
	})
}

// engagementTriggers are the `email.` Events an Automation may start from: what a recipient does.
// The others (sent, delivered, delayed, dropped, bounced, complained) are infrastructure, kept
// for the figures and the log (ADR 0053).
var engagementTriggers = []string{systemevent.EmailOpened, systemevent.EmailClicked, systemevent.EmailUnsubscribed}

// telegramTriggers are the `telegram.` Events an Automation may start from or end on: what happens
// to a Binding and what the person does. `telegram.sent` is infrastructure, like `email.sent`.
var telegramTriggers = []string{
	systemevent.TelegramBound, systemevent.TelegramUnbound, systemevent.TelegramReceived, systemevent.TelegramClicked,
	systemevent.TelegramUnsubscribed,
}

// smsTriggers are the `sms.` Events an Automation may start from or end on: a text the person sent,
// the consent they gave and withdrew, a link they clicked, and whether a text reached their phone
// (a delivery report, which on SMS says the number works or not, unlike email's infrastructure
// notices). The acceptance (`sms.sent`) is infrastructure, like `email.sent`.
var smsTriggers = []string{
	systemevent.SmsReceived, systemevent.SmsConfirmed, systemevent.SmsClicked, systemevent.SmsDelivered, systemevent.SmsUndelivered,
	systemevent.SmsUnsubscribed,
}

// validateTrigger refuses a Trigger whose action is empty or an infrastructure Event, and one
// whose filter does not read as an Event filter (condition.CompileEventFilter): an entry the
// contract's type admits but the filter cannot hold, a property key that is empty, a test a
// property does not take. A filter is bounded as a Rule's Condition is.
func validateTrigger(trigger eventmatcher.Matcher) error {
	if problem := eventProblem(trigger.Action); problem != "" {
		return ErrInvalidTrigger.With(problem)
	}
	if trigger.Filter == nil {
		return nil
	}
	if size := trigger.Filter.Size(); size > rules.MaxConditionNodes {
		return ErrInvalidTrigger.With(fmt.Sprintf("a filter has at most %d groups and tests, this one has %d", rules.MaxConditionNodes, size))
	}
	if _, err := condition.CompileEventFilter(*trigger.Filter); err != nil {
		return ErrInvalidTrigger.With("the filter: " + err.Error())
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

// validateEvents checks the definition's Trigger (validateTrigger), each exit event and the goal
// event by the Trigger's rule (eventProblem), and refuses an exit or goal event equal to the
// Trigger's action and a goal that is also an exit event: one Event would then both enroll and
// end, or end two ways.
func validateEvents(def definition) error {
	if err := validateTrigger(def.trigger); err != nil {
		return err
	}
	trigger := strings.TrimSpace(def.trigger.Action)
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

// normalizeTrigger turns a Trigger into its stored form: its action trimmed.
func normalizeTrigger(t eventmatcher.Matcher) eventmatcher.Matcher {
	t.Action = strings.TrimSpace(t.Action)
	return t
}

// normalizeExits turns a list of exit events into its stored form: each trimmed, the first of a
// repeat kept, never nil.
func normalizeExits(actions []string) []string {
	return lo.Uniq(lo.Map(actions, func(e string, _ int) string { return strings.TrimSpace(e) }))
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
	Name string
	// Trigger is what enrolls a Contact: an Event matcher (ADR 0133).
	Trigger eventmatcher.Matcher
	// ExitEvents end an active Enrollment early and GoalEvent ends it as having reached its goal;
	// nil is none.
	ExitEvents []string
	GoalEvent  *string
	// Steps are the root path; nil is none.
	Steps *automationstep.Path
}

// UpdateInput changes an Automation; nil fields are left alone. The name is the Automation's own;
// the rest is the definition, written to the draft (ADR 0065).
type UpdateInput struct {
	// Revision is the one the edit was made on (ADR 0119).
	Revision int64
	Name     *string
	// Trigger replaces the Trigger, its filter with it; nil leaves it alone.
	Trigger *eventmatcher.Matcher
	// ExitEvents replaces the exit events; nil leaves them alone.
	ExitEvents *[]string
	// GoalEvent sets the goal event, and a clear removes it.
	GoalEvent patch.Field[string]
	// Steps replaces the root path; nil leaves it alone.
	Steps *automationstep.Path
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

// Get returns one Automation of the Workspace, archived or not: an archived Automation is
// still addressable (viewed, unarchived, its Enrollments listed).
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Automation, error) {
	a, err := withVersions(s.Automation().QueryWithArchived().Where(automation.ID(id))).Only(ctx)
	return a, notFound(err)
}

// Create stores a new inactive Automation whose first version is its draft: nothing is live until
// it is published (ADR 0065).
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Automation, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	var steps automationstep.Path
	if in.Steps != nil {
		if steps, err = m.build(ctx, *in.Steps); err != nil {
			return nil, err
		}
	}
	owner := func(ctx context.Context, ts *ent.Scoped) (int64, error) {
		a, err := ts.Automation().Create().SetName(in.Name).Save(ctx)
		if err != nil {
			return 0, err
		}
		return a.ID, nil
	}
	e := edit{trigger: &in.Trigger, exits: &in.ExitEvents, goal: patch.FromPtr(in.GoalEvent)}
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
	name, err := validation.NameOpt(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	e := edit{trigger: in.Trigger, exits: in.ExitEvents, goal: in.GoalEvent}
	if in.Steps != nil {
		built, err := m.build(ctx, *in.Steps)
		if err != nil {
			return nil, err
		}
		e.steps = &built
	}
	return m.life.Edit(ctx, s, id, versioneddefinition.EditInput[edit]{Revision: in.Revision, Name: in.Name, Edit: e})
}

// edit is a change of an Automation's definition; nil fields keep the base's. The steps are built
// (their documents rendered) before the transaction.
type edit struct {
	trigger *eventmatcher.Matcher
	exits   *[]string
	goal    patch.Field[string]
	steps   *automationstep.Path
}

// definition is the content of an Automation version as stored: its Trigger, the events that end
// an Enrollment early (exits) and the one that ends it as having reached its goal (nil for none),
// and its sealed steps, each send step naming its Integration.
type definition struct {
	trigger eventmatcher.Matcher
	exits   []string
	goal    *string
	steps   automationstep.Path
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
	def := definition{exits: []string{}, steps: automationstep.Path{ID: automationstep.NewID(), Steps: []automationstep.Step{}}}
	var stored automationstep.Path
	if base != nil {
		def, stored = definitionOf(base), base.Steps
	}
	if e.trigger != nil {
		def.trigger = normalizeTrigger(*e.trigger)
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
	if e.trigger != nil {
		if err := checkNamed(ctx, ts, def.trigger); err != nil {
			return definition{}, err
		}
	}
	if e.steps != nil {
		linkLength, err := v.m.out.ShortLinkLength(ctx, ts)
		if err != nil {
			return definition{}, err
		}
		// The steps carry their ids: build identified them.
		steps := *e.steps
		if err := validate(steps, linkLength); err != nil {
			return definition{}, err
		}
		if err := checkEnds(steps); err != nil {
			return definition{}, err
		}
		// Each Integration the steps name is checked once, however many steps send through it.
		var named []int64
		for _, st := range automationstep.All(steps) {
			if id := st.Integration(); id != nil {
				named = append(named, *id)
			}
		}
		for _, id := range lo.Uniq(named) {
			if err := checkIntegration(ctx, ts, &id); err != nil {
				return definition{}, err
			}
		}
		sealed, err := v.m.sealSteps(steps, stored)
		if err != nil {
			return definition{}, err
		}
		def.steps = StoredSteps(sealed)
	}
	return def, nil
}

// TriggerOf is the version's Trigger: the Event matcher its action and filter columns hold.
func TriggerOf(v *ent.AutomationVersion) eventmatcher.Matcher {
	return eventmatcher.Matcher{Action: v.TriggerAction, Filter: v.TriggerFilter}
}

// definitionOf is the definition a version holds.
func definitionOf(v *ent.AutomationVersion) definition {
	return definition{trigger: TriggerOf(v), exits: normalizeExits(v.ExitEvents), goal: v.GoalEvent, steps: v.Steps}
}

// Equal compares the Trigger's filter and the steps by value (ADR 0169): a document spelled with its
// keys in another order is the same document.
func (versioned) Equal(v *ent.AutomationVersion, def definition) bool {
	held := definitionOf(v)
	return held.trigger.Action == def.trigger.Action && jsonvalue.Same(held.trigger.Filter, def.trigger.Filter) && slices.Equal(held.exits, def.exits) && optional.Equal(held.goal, def.goal) &&
		jsonvalue.Same(held.steps, def.steps)
}

func (versioned) Write(ctx context.Context, l *ent.WorkspaceLocked, id int64, number int, draft *ent.AutomationVersion, def definition) (int64, error) {
	if draft != nil {
		upd := l.AutomationVersion().UpdateOneID(draft.ID).
			SetTriggerAction(def.trigger.Action).
			SetExitEvents(def.exits).
			SetNillableGoalEvent(def.goal).
			SetSteps(def.steps)
		if def.goal == nil {
			upd.ClearGoalEvent()
		}
		if def.trigger.Filter == nil {
			upd.ClearTriggerFilter()
		} else {
			upd.SetTriggerFilter(def.trigger.Filter)
		}
		return draft.ID, upd.Exec(ctx)
	}
	create := l.AutomationVersion().Create().
		SetAutomationID(id).
		SetVersion(number).
		SetTriggerAction(def.trigger.Action).
		SetExitEvents(def.exits).
		SetNillableGoalEvent(def.goal).
		SetSteps(def.steps)
	if def.trigger.Filter != nil {
		create.SetTriggerFilter(def.trigger.Filter)
	}
	created, err := create.Save(ctx)
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
// Telegram's limit is refused too, and so is an email step while the Workspace has no postal
// address for its footer (outboundmessages.ErrPostalAddressRequired, ADR 0156).
func (v versioned) Publishable(ctx context.Context, ts *ent.Scoped, version *ent.AutomationVersion) error {
	if err := validateEvents(definition{trigger: TriggerOf(version), exits: version.ExitEvents, goal: version.GoalEvent}); err != nil {
		return err
	}
	sends := SendSteps(version)
	if slices.ContainsFunc(sends, func(st SendStep) bool { return st.Channel() == channels.Email }) {
		if err := v.m.out.RequirePostalAddress(ctx, ts, channels.Email); err != nil {
			return err
		}
	}
	for i := range sends {
		st := &sends[i]
		tg, ok := st.Step.Payload.(automationstep.Telegram)
		if !ok {
			continue
		}
		if err := telegramrender.CheckWritten(tg.Text, tg.ImageURL != ""); err != nil {
			return refuse(&st.Step, err.Error())
		}
	}
	type routeKey struct {
		ch          channels.Channel
		integration int64
	}
	asked := map[routeKey]bool{}
	for i := range sends {
		st := &sends[i]
		key := routeKey{st.Channel(), lo.FromPtr(st.Step.Integration())}
		if asked[key] {
			continue
		}
		asked[key] = true
		if err := checkIntegration(ctx, ts, st.Step.Integration()); err != nil {
			return err
		}
		route, err := v.m.out.Route(ctx, ts, st.Channel(), st.Step.Integration())
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
func (m *Module) Senders(ctx context.Context, s *ent.Scoped) ([]*ent.Automation, error) {
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
		if len(SendSteps(a.Edges.LiveVersion)) > 0 {
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
func (v versioned) Archivable(ctx context.Context, l *ent.WorkspaceLocked, a *ent.Automation) error {
	enrolling, err := v.m.rules.ActiveEnrollingInto(ctx, l, a.ID)
	if err != nil {
		return err
	}
	if len(enrolling) > 0 {
		return &liveuse.Error{What: liveuse.SubjectAutomation, Users: liveuse.Rules(enrolling)}
	}
	active, err := l.AutomationRun().Query().
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

// checkNamed refuses a Trigger whose filter names a Tag or Custom field the Workspace's catalogue
// lacks (condition.NamedDefinitions, ADR 0133): such a filter would never match. A Tag is named
// as the Event carries it, by its stored name, so another spelling of it is refused too.
func checkNamed(ctx context.Context, s *ent.Scoped, trigger eventmatcher.Matcher) error {
	if trigger.Filter == nil {
		return nil
	}
	named := condition.NamedDefinitions(trigger.Action, *trigger.Filter)
	for _, name := range named.Tags {
		ok, err := s.Tag().Query().Where(tag.Name(name)).Exist(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidTrigger.With(fmt.Sprintf("the filter names the Tag %q, which the Workspace does not have", name))
		}
	}
	for _, key := range named.CustomFields {
		ok, err := s.CustomField().Query().Where(customfield.Key(key)).Exist(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return ErrInvalidTrigger.With(fmt.Sprintf("the filter names the Custom field %q, which the Workspace does not have", key))
		}
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
func (m *Module) Publish(ctx context.Context, s *ent.Scoped, id, revision int64) (*ent.Automation, error) {
	return m.life.Publish(ctx, s, id, revision)
}

// Revert drops the draft: the live version is again the Automation's latest. It is refused
// unless there is a draft above a live version, since a never-published Automation's draft is
// its whole definition. Revert is authoring. An archived Automation is read-only: ErrArchived.
func (m *Module) Revert(ctx context.Context, s *ent.Scoped, id, revision int64) (*ent.Automation, error) {
	return m.life.Revert(ctx, s, id, revision)
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
