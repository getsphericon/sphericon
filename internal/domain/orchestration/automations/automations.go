// Package automations is the one place an Automation's definition is stored,
// validated and its lifecycle changed (ADR 0016). /site and /api handlers stay thin
// adapters over it. internal/kernel/automationstep owns the stored Step format and its reader,
// shared by authoring, live-use checks and execution. The definition (Trigger, exit and goal events and
// steps, each send step with its own Integration, ADR 0081) lives in versions (ADR 0049): an edit
// writes the draft, Publish makes it live (ADR 0065). StepExecution walks the Enrollments through
// their steps (ADR 0112).
package automations

import (
	"cmp"
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
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/orders"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/content/versioneddefinition"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/messagesenders"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/sending/linkclick"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/datetrigger"
	"github.com/getsphericon/sphericon/internal/kernel/emaildoc"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/jsonvalue"
	"github.com/getsphericon/sphericon/internal/kernel/localcalendar"
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
	// ErrInvalidExitEvent: an exit event's action is empty or an infrastructure Event, or its filter
	// does not read as an Event filter or names a Tag or Custom field the Workspace lacks.
	ErrInvalidExitEvent = validation.Refuse("automations: invalid exit event", i18n.ErrorsAutomationExitEventInvalid)
	// ErrInvalidGoalEvent: the goal event's action is empty or an infrastructure Event, or its filter
	// does not read as an Event filter or names a Tag or Custom field the Workspace lacks.
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
	// ErrUnknownSender: an event filter of an Automation names a Broadcast or an Automation the Workspace
	// does not have (ADR 0153), an unknown reference. The message names the test's path and the id.
	ErrUnknownSender = validation.UnknownReference("automations: unknown sender", i18n.ErrorsAutomationSenderUnknown)

	// ErrNoVersion: the Automation has no live version: it was never published, so nobody can
	// be enrolled in it.
	ErrNoVersion = validation.Conflict("automations: automation has no live version", i18n.ErrorsAutomationNoVersion)
	// ErrInactive: the Automation a Rule enrolls into is inactive or archived by the time the
	// Enrollment is made, so it enrolls nobody (StepExecution.EnrollOnRule).
	ErrInactive = validation.Conflict("automations: automation is not active", i18n.ErrorsAutomationInactive)
	// ErrDateTriggerEnrolledInto: a version with a date Trigger cannot go live while an active Rule
	// enrolls into the Automation (ADR 0126). The message names the Rules.
	ErrDateTriggerEnrolledInto = validation.Conflict("automations: a rule enrolls into the automation", i18n.ErrorsAutomationDateTriggerEnrolledInto)

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

// MaxStepsPerVersion bounds a version's tree (ADR 0122, ADR 0124): its Steps over all paths. The
// paths of one fork are the contract's (bounds.BranchPaths, bounds.SplitPaths); nesting has no bound
// of its own.
const MaxStepsPerVersion = 100

// validate checks each step on its own, at any depth of the tree, and the tree's bounds; the steps
// carry their ids (identify). linkLength is the length of the Workspace's short tracked links, what
// an SMS step's links count as.
func validate(steps automationstep.Path, linkLength int) error {
	for pos, st := range automationstep.All(steps) {
		if pos >= MaxStepsPerVersion {
			return refuse(st, fmt.Sprintf("a version has at most %d steps over all its paths", MaxStepsPerVersion))
		}
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
		case automationstep.Branch:
			if err := validateBranch(st, s); err != nil {
				return err
			}
		case automationstep.Split:
			if err := validateSplit(st, s); err != nil {
				return err
			}
		default:
			return refuse(st, "the step has no known kind")
		}
	}
	return nil
}

// validateBranch checks the shape of a Branch (ADR 0124): the contract's count of listed paths
// besides otherwise (bounds.BranchPaths), and a Condition that tests something on each path.
// Whether each Condition names what the Workspace holds is checkBranches'.
func validateBranch(st *automationstep.Step, b automationstep.Branch) error {
	if n := len(b.Paths); n < bounds.BranchPathsMinItems || n > bounds.BranchPathsMaxItems {
		return refuse(st, fmt.Sprintf("a branch has %d to %d paths besides otherwise, this one has %d",
			bounds.BranchPathsMinItems, bounds.BranchPathsMaxItems, n))
	}
	for i, p := range b.Paths {
		if !p.Condition.Tests() {
			return refuse(st, fmt.Sprintf("path %d has no condition", i+1))
		}
	}
	return nil
}

// NeedsUnknownPath reports whether a Branch whose listed paths hold these Conditions needs an
// unknown path (ADR 0124): it does when one of them can be unknown for a Contact
// (condition.CanBeUnknown: an engagement count, ADR 0123). Publish arms a Branch by it, and the
// editor asks it of the Conditions being edited.
func NeedsUnknownPath(conditions []conditiontree.Branch) bool {
	return slices.ContainsFunc(conditions, func(c conditiontree.Branch) bool { return condition.CanBeUnknown(c.Group) })
}

// checkUnknownPaths refuses, naming it, a Branch of the tree whose Conditions can be unknown for a
// Contact (condition.CanBeUnknown: an engagement count, ADR 0123) and that has no unknown path, and
// a Branch whose Conditions cannot be unknown and that has one (ADR 0124). A save keeps either, so
// the author can build the Branch up; Publish arms it.
func checkUnknownPaths(steps automationstep.Path) error {
	for _, st := range automationstep.All(steps) {
		b, ok := st.Payload.(automationstep.Branch)
		if !ok {
			continue
		}
		needs := NeedsUnknownPath(b.Conditions())
		switch {
		case needs && b.Unknown == nil:
			return refuse(st, "a branch whose conditions count an open or a click needs an unknown path, taken when one is unknown for the contact")
		case !needs && b.Unknown != nil:
			return refuse(st, "a branch whose conditions count no open or click is never unknown, so it takes no unknown path")
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
	// tags and people read the Tag and Custom field catalogues an Event filter names (checkNamed).
	tags   *tags.Module
	people *contacts.Module
	// conditions checks a Branch path's Condition (checkBranches).
	conditions *condition.Module
	// senders answers for the Broadcasts and Automations an Event filter or a Branch path's event
	// test names (checkNamed, checkBranches).
	senders *messagesenders.Module
	life    *versioneddefinition.Lifecycle[ent.Automation, ent.AutomationVersion, edit, definition]
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
// the Rules that enroll Contacts in an Automation, which hold it against its Archive; people is the
// contacts module, whose Custom field catalogue an Event filter names; now is the Instance clock.
func New(cipher *secrets.Cipher, bus *events.Bus, renderer *emaildoc.Renderer, out *outboundmessages.Module, clicks *linkclick.Module, enrollers *rules.Module, people *contacts.Module, now func() time.Time) *Module {
	m := &Module{cipher: cipher, bus: bus, renderer: renderer, out: out, clicks: clicks, rules: enrollers, people: people, now: now,
		revenues: orders.NewRevenues(), tags: tags.New(), conditions: condition.New(now), senders: messagesenders.New()}
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

// validateSplit checks the shape of a Split (ADR 0124): the contract's count of paths
// (bounds.SplitPaths) and least weight (bounds.SplitWeight), and the weights summing to 100.
func validateSplit(st *automationstep.Step, sp automationstep.Split) error {
	if n := len(sp.Paths); n < bounds.SplitPathsMinItems || n > bounds.SplitPathsMaxItems {
		return refuse(st, fmt.Sprintf("a split has %d to %d paths, this one has %d", bounds.SplitPathsMinItems, bounds.SplitPathsMaxItems, n))
	}
	sum := 0
	for i, p := range sp.Paths {
		if p.Weight < bounds.SplitWeightMin {
			return refuse(st, fmt.Sprintf("path %d: a weight is at least %d, this one is %d", i+1, bounds.SplitWeightMin, p.Weight))
		}
		sum += p.Weight
	}
	if sum != splitWhole {
		return refuse(st, fmt.Sprintf("the weights of a split sum to %d, these sum to %d", splitWhole, sum))
	}
	return nil
}

// splitWhole is what a Split's weights sum to: each is a percentage of the Contacts that reach it.
const splitWhole = 100

// checkBranches checks the Condition of each path of each Branch of the tree as a Rule's is
// checked (condition.Module.CheckBranch), bounded as a Rule's: it reads as a Branch path's tree,
// and names only the Custom fields the Workspace holds, by tests their types take. A refusal names
// the Branch and the path. A Condition whose event test's filter names a Broadcast or an Automation
// the Workspace does not have (messagesenders) is ErrUnknownSender (ADR 0153).
func (m *Module) checkBranches(ctx context.Context, s *ent.Scoped, steps automationstep.Path) error {
	for _, st := range automationstep.All(steps) {
		b, ok := st.Payload.(automationstep.Branch)
		if !ok {
			continue
		}
		for i, p := range b.Paths {
			if size := p.Condition.Size(); size > rules.MaxConditionNodes {
				return refuse(st, fmt.Sprintf("path %d: a condition has at most %d groups and tests, this one has %d", i+1, rules.MaxConditionNodes, size))
			}
			err := m.conditions.CheckBranch(ctx, s, p.Condition)
			if errors.Is(err, condition.ErrStorage) {
				return err
			}
			if err != nil {
				return refuse(st, fmt.Sprintf("path %d: %s", i+1, err.Error()))
			}
			unknown, err := m.senders.Unknown(ctx, s, condition.NamedSenders(p.Condition.Group))
			if err != nil {
				return err
			}
			if unknown != nil {
				return ErrUnknownSender.With(fmt.Sprintf("step %s (%s): path %d: %s", st.ID, st.Kind(), i+1, unknown.Missing()))
			}
		}
	}
	return nil
}

// engagementTriggers are the `email.` Events an Automation may start from: what a recipient does,
// and what they decided about the tracking of their address (ADR 0120). The others (sent,
// delivered, delayed, dropped, bounced, complained) are infrastructure, kept for the figures and
// the log (ADR 0053).
var engagementTriggers = []string{
	systemevent.EmailOpened, systemevent.EmailClicked, systemevent.EmailUnsubscribed,
	systemevent.EmailEngagementTrackingGranted, systemevent.EmailEngagementTrackingDenied,
}

// telegramTriggers are the `telegram.` Events an Automation may start from or end on: what happens
// to a Binding, what the person does and what they decided about the tracking of their chat (ADR
// 0120). `telegram.sent` is infrastructure, like `email.sent`.
var telegramTriggers = []string{
	systemevent.TelegramBound, systemevent.TelegramUnbound, systemevent.TelegramReceived, systemevent.TelegramClicked,
	systemevent.TelegramUnsubscribed, systemevent.TelegramEngagementTrackingGranted, systemevent.TelegramEngagementTrackingDenied,
}

// smsTriggers are the `sms.` Events an Automation may start from or end on: a text the person sent,
// the consent they gave and withdrew, the tracking of their number they agreed to or refused (ADR
// 0120), a link they clicked, and whether a text reached their phone (a delivery report, which on
// SMS says the number works or not, unlike email's infrastructure notices). The acceptance
// (`sms.sent`) is infrastructure, like `email.sent`.
var smsTriggers = []string{
	systemevent.SmsReceived, systemevent.SmsConfirmed, systemevent.SmsClicked, systemevent.SmsDelivered, systemevent.SmsUndelivered,
	systemevent.SmsUnsubscribed, systemevent.SmsEngagementTrackingGranted, systemevent.SmsEngagementTrackingDenied,
}

// matcherProblem says why an Event matcher (ADR 0133) can neither start nor end an Automation: its
// action is empty or an infrastructure Event (eventProblem), or its filter does not read as an Event
// filter (condition.CompileEventFilter): an entry the contract's type admits but the filter cannot
// hold, a property key that is empty, a test a property does not take. A filter is bounded as a
// Rule's Condition is. Empty when it can.
func matcherProblem(m eventmatcher.Matcher) string {
	if problem := eventProblem(m.Action); problem != "" {
		return problem
	}
	if m.Filter == nil {
		return ""
	}
	if size := m.Filter.Size(); size > rules.MaxConditionNodes {
		return fmt.Sprintf("a filter has at most %d groups and tests, this one has %d", rules.MaxConditionNodes, size)
	}
	if _, err := condition.CompileEventFilter(m.Action, *m.Filter); err != nil {
		return "the filter: " + err.Error()
	}
	return ""
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

// validateEvents checks each Event matcher of the definition (matcherProblem): its Trigger, its
// exit events and its goal event. They may share an action (ADR 0133): an Enrollment's triggering
// Event never ends it, and an Event matching the goal and an exit reaches the goal.
func validateEvents(def definition) error {
	if def.trigger.Date == nil {
		if problem := matcherProblem(def.trigger.Event); problem != "" {
			return ErrInvalidTrigger.With(problem)
		}
	}
	for _, e := range def.exits {
		if problem := matcherProblem(e); problem != "" {
			return ErrInvalidExitEvent.With(problem)
		}
	}
	if def.goal == nil {
		return nil
	}
	if problem := matcherProblem(*def.goal); problem != "" {
		return ErrInvalidGoalEvent.With(problem)
	}
	return nil
}

// Trigger is what enrolls a Contact in an Automation, one of two kinds: an Event matcher (ADR
// 0133), Event, when Date is nil; or a date the Contact holds (ADR 0126), Date, with Event left
// empty. A date Trigger takes no Entry policy, cooldown or Enrollment key.
type Trigger struct {
	Event eventmatcher.Matcher
	Date  *datetrigger.Date
}

// normalizeTrigger turns a Trigger into its stored form: an Event matcher normalized, and a date
// Trigger with no matcher.
func normalizeTrigger(t Trigger) Trigger {
	if t.Date != nil {
		d := *t.Date
		d.Field.Key = strings.TrimSpace(d.Field.Key)
		return Trigger{Date: &d}
	}
	return Trigger{Event: normalizeMatcher(t.Event)}
}

// sameTrigger reports whether two Triggers enroll alike: the same matcher, or the same date.
func sameTrigger(a, b Trigger) bool {
	return sameMatcher(a.Event, b.Event) && optional.Equal(a.Date, b.Date)
}

// errDateTriggerEntry is the refusal of an Entry policy, a cooldown or an Enrollment key given with
// a date Trigger, which has none (ADR 0126).
const errDateTriggerEntry = "a date trigger takes no entry policy, cooldown or enrollment key"

// dateProblem says why a date Trigger cannot be stored, by the rules that need no catalogue: its
// schedule out of the calendar's bounds (localcalendar.NewSchedule), a field of no known kind or a
// Custom field without a key, and `once` on the birthday, which is always past (decision 5 of
// #1613). Empty when it can.
func dateProblem(d datetrigger.Date) string {
	if _, err := d.Schedule(); err != nil {
		return err.Error()
	}
	switch d.Field.Kind {
	case datetrigger.Birthday:
		if d.Recurrence == localcalendar.Once {
			return "a birthday is always past, so its trigger is only yearly"
		}
		return ""
	case datetrigger.CustomField:
		if d.Field.Key == "" {
			return "the custom field has no key"
		}
		return ""
	}
	return fmt.Sprintf("%q is not a date field", d.Field.Kind)
}

// checkDate refuses a date Trigger dateProblem refuses, or whose Custom field the Workspace's
// catalogue lacks or holds with another type than datetime: such a Trigger would never fire.
func (m *Module) checkDate(ctx context.Context, s *ent.Scoped, d datetrigger.Date) error {
	if problem := dateProblem(d); problem != "" {
		return ErrInvalidTrigger.With(problem)
	}
	if d.Field.Kind != datetrigger.CustomField {
		return nil
	}
	field, err := m.people.CustomFieldByKey(ctx, s, d.Field.Key)
	if err != nil {
		return err
	}
	if field == nil {
		return ErrInvalidTrigger.With(fmt.Sprintf("the Workspace has no Custom field %q", d.Field.Key))
	}
	if field.Type != customfield.TypeDatetime {
		return ErrInvalidTrigger.With(fmt.Sprintf("the Custom field %q is a %s, not a datetime", d.Field.Key, field.Type))
	}
	return nil
}

// normalizeMatcher turns an Event matcher into its stored form: its action trimmed.
func normalizeMatcher(m eventmatcher.Matcher) eventmatcher.Matcher {
	m.Action = strings.TrimSpace(m.Action)
	return m
}

// normalizeExits turns a list of exit events into its stored form: each normalized, the first of a
// repeat (the same action and the same filter) kept, never nil.
func normalizeExits(exits []eventmatcher.Matcher) []eventmatcher.Matcher {
	out := []eventmatcher.Matcher{}
	for _, e := range exits {
		e = normalizeMatcher(e)
		if !slices.ContainsFunc(out, func(kept eventmatcher.Matcher) bool { return sameMatcher(kept, e) }) {
			out = append(out, e)
		}
	}
	return out
}

// normalizeGoal turns a goal event into its stored form: normalized, nil for none.
func normalizeGoal(goal *eventmatcher.Matcher) *eventmatcher.Matcher {
	if goal == nil {
		return nil
	}
	g := normalizeMatcher(*goal)
	return &g
}

// Entry is how a version's Event Trigger enrolls (ADR 0127, ADR 0165): Policy, `once` (the
// default, empty) or `again`; CooldownSeconds, under `again` the least time from the beginning of a
// Contact's previous Enrollment to the next, nil for the default and at least an hour; and Key, the
// Enrollment key, the name of a property of the triggering Event by whose value a Contact may have
// several active Enrollments, nil for none.
type Entry struct {
	Policy          automationversion.EntryPolicy
	CooldownSeconds *int
	Key             *string
}

// given reports whether the Entry sets anything, which only an Event Trigger takes.
func (e Entry) given() bool {
	return e.Policy != "" || e.CooldownSeconds != nil || e.Key != nil
}

// defaultEntry is the Entry settings a version stores with no Entry given: `once`, the default
// cooldown and no Enrollment key. A date Trigger stores them.
var defaultEntry = entrySettings{policy: automationversion.DefaultEntryPolicy, cooldown: bounds.TriggerCooldownDefault}

// normalizeEntry turns an Entry into its stored form, the defaults filled in, and refuses a
// cooldown under the floor (bounds.EntryCooldown) and an Enrollment key that is no property name
// (condition.CheckPropertyKey, the form a filter's keys have).
func normalizeEntry(e Entry) (entrySettings, error) {
	policy := cmp.Or(e.Policy, automationversion.DefaultEntryPolicy)
	if err := automationversion.EntryPolicyValidator(policy); err != nil {
		return entrySettings{}, ErrInvalidTrigger.With(fmt.Sprintf("%q is not an entry policy", policy))
	}
	cooldown := bounds.TriggerCooldownDefault
	if e.CooldownSeconds != nil {
		cooldown = *e.CooldownSeconds
	}
	if err := validation.Min(fieldname.Cooldown, cooldown, bounds.EntryCooldownMin); err != nil {
		return entrySettings{}, err
	}
	if e.Key != nil {
		if err := condition.CheckPropertyKey(*e.Key); err != nil {
			return entrySettings{}, ErrInvalidTrigger.With("the enrollment key: " + err.Error())
		}
	}
	return entrySettings{policy: policy, cooldown: cooldown, key: e.Key}, nil
}

// entrySettings is an Entry as a version stores it: the policy, the cooldown in seconds and the
// Enrollment key, nil for none.
type entrySettings struct {
	policy   automationversion.EntryPolicy
	cooldown int
	key      *string
}

// sameMatcher reports whether two Event matchers match the same Events by what they say: one action
// and the same filter by value (ADR 0169).
func sameMatcher(a, b eventmatcher.Matcher) bool {
	return a.Action == b.Action && jsonvalue.Same(a.Filter, b.Filter)
}

// CreateInput is the data for a new Automation. It is always created inactive: publishing is a
// separate, send-class operation (ADR 0065).
type CreateInput struct {
	Name string
	// Trigger is what enrolls a Contact: an Event matcher (ADR 0133) or a date (ADR 0126).
	Trigger Trigger
	// Entry is an Event Trigger's Entry policy; its zero value is the defaults, and a date Trigger
	// takes no other.
	Entry Entry
	// ExitEvents end an active Enrollment early and GoalEvent ends it as having reached its goal,
	// each an Event matcher (ADR 0133); nil is none.
	ExitEvents []eventmatcher.Matcher
	GoalEvent  *eventmatcher.Matcher
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
	Trigger *Trigger
	// Entry replaces the Trigger's Entry policy, which a request writes with the Trigger; nil
	// leaves it alone.
	Entry *Entry
	// ExitEvents replaces the exit events; nil leaves them alone.
	ExitEvents *[]eventmatcher.Matcher
	// GoalEvent sets the goal event, and a clear removes it.
	GoalEvent patch.Field[eventmatcher.Matcher]
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
	e := edit{trigger: &in.Trigger, entry: &in.Entry, exits: &in.ExitEvents, goal: patch.FromPtr(in.GoalEvent)}
	if in.Steps != nil {
		e.steps = &steps
	}
	return m.life.Create(ctx, s, owner, e)
}

// Update renames the Automation and writes its definition to the draft (ADR 0065,
// internal/domain/content/versioneddefinition): the fields left out keep the draft's, or the live version's when
// there is no draft. The live version is never rewritten: the first edit above it creates the
// draft, the next ones rewrite the draft in place. A definition equal to the draft's (or, with no
// draft, the live version's) stores nothing. An edit of the Trigger, exit or goal events checks
// each matcher (validateEvents); they may share an action. The status is untouched. An archived Automation is read-only: ErrArchived.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Automation, error) {
	name, err := validation.NameOpt(in.Name)
	if err != nil {
		return nil, err
	}
	in.Name = name
	e := edit{trigger: in.Trigger, entry: in.Entry, exits: in.ExitEvents, goal: in.GoalEvent}
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
	trigger *Trigger
	entry   *Entry
	exits   *[]eventmatcher.Matcher
	goal    patch.Field[eventmatcher.Matcher]
	steps   *automationstep.Path
}

// definition is the content of an Automation version as stored: its Trigger with its Entry policy,
// cooldown in seconds and Enrollment key, the Event matchers that end an Enrollment early (exits) and the one that
// ends it as having reached its goal (nil for none), and its sealed steps, each send step naming its
// Integration.
type definition struct {
	trigger Trigger
	entry   entrySettings
	exits   []eventmatcher.Matcher
	goal    *eventmatcher.Matcher
	steps   automationstep.Path
}

// versioned is the Automation's part of the lifecycle (versioneddefinition.Definition).
type versioned struct{ m *Module }

func (versioned) Store(s *ent.Scoped) ent.VersionedStore[ent.Automation, ent.AutomationVersion] {
	return s.Automation().Pointers()
}

// Merge lays the edit over the base version (nil for a new Automation) and validates what it
// changes: the Trigger, exit and goal events (validateEvents, and the definitions their filters
// name, checkNamed), and the steps, each send step's Integration kept (not archived). An HTTP step that names one of the
// base's steps by ID keeps the secrets it does not resend.
func (v versioned) Merge(ctx context.Context, ts *ent.Scoped, base *ent.AutomationVersion, e edit) (definition, error) {
	def := definition{
		entry: defaultEntry,
		exits: []eventmatcher.Matcher{}, steps: automationstep.Path{ID: automationstep.NewID(), Steps: []automationstep.Step{}},
	}
	var stored automationstep.Path
	if base != nil {
		def, stored = definitionOf(base), base.Steps
	}
	if e.trigger != nil {
		def.trigger = normalizeTrigger(*e.trigger)
	}
	if def.trigger.Date != nil {
		// A date Trigger has no Entry policy, cooldown or Enrollment key: it stores the defaults.
		if e.entry != nil && e.entry.given() {
			return definition{}, ErrInvalidTrigger.With(errDateTriggerEntry)
		}
		def.entry = defaultEntry
	} else if e.entry != nil {
		settings, err := normalizeEntry(*e.entry)
		if err != nil {
			return definition{}, err
		}
		def.entry = settings
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
		if err := v.m.checkTrigger(ctx, ts, def.trigger); err != nil {
			return definition{}, err
		}
	}
	if e.exits != nil {
		for _, exit := range def.exits {
			if err := v.m.checkNamed(ctx, ts, exit, ErrInvalidExitEvent); err != nil {
				return definition{}, err
			}
		}
	}
	if e.goal.Changes() && def.goal != nil {
		if err := v.m.checkNamed(ctx, ts, *def.goal, ErrInvalidGoalEvent); err != nil {
			return definition{}, err
		}
	}
	if e.steps != nil {
		steps, err := v.mergeSteps(ctx, ts, *e.steps, stored)
		if err != nil {
			return definition{}, err
		}
		def.steps = steps
	}
	return def, nil
}

// mergeSteps validates the steps an edit writes, each send step's Integration kept (not archived),
// and seals them over the stored ones: an HTTP step that names one of them by ID keeps the secrets
// it does not resend. The steps carry their ids: build identified them.
func (v versioned) mergeSteps(ctx context.Context, ts *ent.Scoped, steps, stored automationstep.Path) (automationstep.Path, error) {
	linkLength, err := v.m.out.ShortLinkLength(ctx, ts)
	if err != nil {
		return automationstep.Path{}, err
	}
	if err := validate(steps, linkLength); err != nil {
		return automationstep.Path{}, err
	}
	if err := checkEnds(steps); err != nil {
		return automationstep.Path{}, err
	}
	if err := v.m.checkBranches(ctx, ts, steps); err != nil {
		return automationstep.Path{}, err
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
			return automationstep.Path{}, err
		}
	}
	sealed, err := v.m.sealSteps(steps, stored)
	if err != nil {
		return automationstep.Path{}, err
	}
	return StoredSteps(sealed), nil
}

// TriggerOf is the version's Event Trigger: the Event matcher its action and filter columns hold,
// empty for a date Trigger (DateTriggerOf).
func TriggerOf(v *ent.AutomationVersion) eventmatcher.Matcher {
	return eventmatcher.Matcher{Action: v.TriggerAction, Filter: v.TriggerFilter}
}

// DateTriggerOf is the version's date Trigger (ADR 0126), nil for an Event Trigger.
func DateTriggerOf(v *ent.AutomationVersion) *datetrigger.Date {
	return v.TriggerDate
}

// checkTrigger refuses a Trigger whose catalogue references do not hold: an Event matcher's filter
// (checkNamed), or a date's Custom field (checkDate).
func (m *Module) checkTrigger(ctx context.Context, s *ent.Scoped, t Trigger) error {
	if t.Date != nil {
		return m.checkDate(ctx, s, *t.Date)
	}
	return m.checkNamed(ctx, s, t.Event, ErrInvalidTrigger)
}

// ExitsOf is the version's exit events, the Event matchers that end an Enrollment on it early;
// never nil.
func ExitsOf(v *ent.AutomationVersion) []eventmatcher.Matcher {
	return normalizeExits(v.Exits)
}

// definitionOf is the definition a version holds.
func definitionOf(v *ent.AutomationVersion) definition {
	return definition{
		trigger: Trigger{Event: TriggerOf(v), Date: DateTriggerOf(v)},
		entry:   entrySettings{policy: v.EntryPolicy, cooldown: v.CooldownSeconds, key: v.EnrollmentKey},
		exits:   ExitsOf(v), goal: v.Goal, steps: v.Steps,
	}
}

// Equal compares the matchers' filters and the steps by value (ADR 0169): a document spelled with
// its keys in another order is the same document.
func (versioned) Equal(v *ent.AutomationVersion, def definition) bool {
	held := definitionOf(v)
	return sameTrigger(held.trigger, def.trigger) &&
		held.entry.policy == def.entry.policy && held.entry.cooldown == def.entry.cooldown && optional.Equal(held.entry.key, def.entry.key) &&
		slices.EqualFunc(held.exits, def.exits, sameMatcher) &&
		optional.EqualFunc(held.goal, def.goal, sameMatcher) &&
		jsonvalue.Same(held.steps, def.steps)
}

func (versioned) Write(ctx context.Context, l *ent.WorkspaceLocked, id int64, number int, draft *ent.AutomationVersion, def definition) (int64, error) {
	if draft != nil {
		upd := l.AutomationVersion().UpdateOneID(draft.ID).
			SetTriggerAction(def.trigger.Event.Action).
			SetEntryPolicy(def.entry.policy).
			SetCooldownSeconds(def.entry.cooldown).
			SetExits(def.exits).
			SetSteps(def.steps)
		if def.trigger.Date == nil {
			upd.ClearTriggerDate()
		} else {
			upd.SetTriggerDate(def.trigger.Date)
		}
		if def.entry.key == nil {
			upd.ClearEnrollmentKey()
		} else {
			upd.SetEnrollmentKey(*def.entry.key)
		}
		if def.goal == nil {
			upd.ClearGoal()
		} else {
			upd.SetGoal(def.goal)
		}
		if def.trigger.Event.Filter == nil {
			upd.ClearTriggerFilter()
		} else {
			upd.SetTriggerFilter(def.trigger.Event.Filter)
		}
		return draft.ID, upd.Exec(ctx)
	}
	create := l.AutomationVersion().Create().
		SetAutomationID(id).
		SetVersion(number).
		SetTriggerAction(def.trigger.Event.Action).
		SetEntryPolicy(def.entry.policy).
		SetCooldownSeconds(def.entry.cooldown).
		SetNillableEnrollmentKey(def.entry.key).
		SetExits(def.exits).
		SetSteps(def.steps)
	if def.trigger.Event.Filter != nil {
		create.SetTriggerFilter(def.trigger.Event.Filter)
	}
	if def.trigger.Date != nil {
		create.SetTriggerDate(def.trigger.Date)
	}
	if def.goal != nil {
		create.SetGoal(def.goal)
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
// address for its footer (outboundmessages.ErrPostalAddressRequired, ADR 0156). A Branch needs an
// unknown path exactly when its Conditions can be unknown (checkUnknownPaths).
func (v versioned) Publishable(ctx context.Context, l *ent.WorkspaceLocked, version *ent.AutomationVersion) error {
	ts := l.Scope()
	if err := validateEvents(definitionOf(version)); err != nil {
		return err
	}
	if date := DateTriggerOf(version); date != nil {
		if err := v.m.checkDate(ctx, ts, *date); err != nil {
			return err
		}
		if err := v.m.refuseEnrollers(ctx, l, version.AutomationID); err != nil {
			return err
		}
	}
	if err := checkUnknownPaths(version.Steps); err != nil {
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

// refuseEnrollers refuses to publish a date Trigger on the Automation while an active Rule enrolls
// into it through its live version (decision 4 of #1613): an Enrollment no occurrence made would
// count against no occurrence. The Rules are read as the Archive refusal reads them, under the
// Workspace lock, so a Rule's Publish, which refuses a date-triggered Automation under the same
// lock, cannot slip past it.
func (m *Module) refuseEnrollers(ctx context.Context, l *ent.WorkspaceLocked, automationID int64) error {
	enrolling, err := m.rules.ActiveEnrollingInto(ctx, l, automationID)
	if err != nil {
		return err
	}
	if len(enrolling) == 0 {
		return nil
	}
	names := lo.Map(enrolling, func(r *ent.Rule, _ int) string { return fmt.Sprintf("%q", r.Name) })
	return ErrDateTriggerEnrolledInto.With("enrolled into by the active rules " + strings.Join(names, ", "))
}

// Published resets the date scheduler's cursor of the Automation when the version made live has a
// date Trigger (ADR 0126, decision 1 of #1613): its active span starts at this Publish, the later
// of the activation and the Publish, and its watermark stands there, so no occurrence before the
// Publish enrolls anyone or counts as missed.
func (v versioned) Published(ctx context.Context, l *ent.WorkspaceLocked, version *ent.AutomationVersion) error {
	if DateTriggerOf(version) == nil {
		return nil
	}
	return resetCursor(ctx, l.Scope(), version.AutomationID, v.m.now())
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

// checkNamed refuses, with refusal, an Event matcher whose filter names a Tag or Custom field the
// Workspace's catalogue lacks (condition.NamedDefinitions, ADR 0133): such a filter would never
// match. A Tag is named as the Event carries it, by its stored name, so another spelling of it is
// refused too. The catalogues are their owners' (tags, contacts). A filter that names a Broadcast
// or an Automation the Workspace does not have, archived ones being its own (messagesenders), is
// ErrUnknownSender (ADR 0153).
func (m *Module) checkNamed(ctx context.Context, s *ent.Scoped, matcher eventmatcher.Matcher, refusal *validation.RefusalError) error {
	if matcher.Filter == nil {
		return nil
	}
	unknown, err := m.senders.Unknown(ctx, s, condition.FilterSenders(*matcher.Filter))
	if err != nil {
		return err
	}
	if unknown != nil {
		return ErrUnknownSender.With("the filter: " + unknown.Missing())
	}
	named := condition.NamedDefinitions(matcher.Action, *matcher.Filter)
	for _, name := range named.Tags {
		ok, err := m.tags.Stored(ctx, s, name)
		if err != nil {
			return err
		}
		if !ok {
			return refusal.With(fmt.Sprintf("the filter names the Tag %q, which the Workspace does not have", name))
		}
	}
	for _, key := range named.CustomFields {
		ok, err := m.people.HasCustomField(ctx, s, key)
		if err != nil {
			return err
		}
		if !ok {
			return refusal.With(fmt.Sprintf("the filter names the Custom field %q, which the Workspace does not have", key))
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
