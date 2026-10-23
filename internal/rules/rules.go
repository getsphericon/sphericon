// Package rules is the Rule module (ADR 0054): a stateless reaction to an Event. When any of a
// Rule's Trigger events is recorded for a Contact and its Condition holds, its Actions run at
// once, in order. The module writes and validates Rules, evaluates an Event against the active
// ones (Evaluate, the entry both the evaluation job and the tests call, no queue needed) and runs
// a Firing's Actions (RunActions).
//
// It takes a *ent.Scoped on every call and holds no client. It imports none of the modules the
// Actions call: an Action is a Runner supplied by the caller (internal/jobs), so adding an
// Action kind is a new Runner and a new type name here, nothing in the evaluation.
package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/riverqueue/river"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/rule"
	"github.com/getsphericon/sphericon/ent/rulefiring"
	"github.com/getsphericon/sphericon/ent/ruletrigger"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// The fixed caps on a Rule (ADR 0054). There is none on the number of Rules.
const (
	// MaxActions is how many Actions one Rule may run.
	MaxActions = 20
	// MaxConditionNodes is how many nodes (groups and rules, nested ones included) a Condition
	// may have.
	MaxConditionNodes = 50
)

// The Action types. Each has a Runner (ActionRunners) that does it.
const (
	ActionApplyTag  = "apply_tag"
	ActionRemoveTag = "remove_tag"
	// ActionEnroll enrolls the Contact in an Automation (once ever, ADR 0046).
	ActionEnroll = "enroll_automation"
	// ActionHTTP calls a URL with a body rendered for the Contact and the Event.
	ActionHTTP = "http"
)

// MaxHTTPActionsPerRule keeps one Rule from queueing an unbounded number of outbound calls.
const MaxHTTPActionsPerRule = 5

// Domain errors returned by Module. Callers match with errors.Is.
var (
	// ErrInvalid: the Rule does not fit the rules of a Rule (no Trigger event, an unknown
	// Action, a tag Action without a tag, a Condition that does not parse or does not fit
	// the Contact schema). The wrapped message is safe to show to the author.
	ErrInvalid = errors.New("rules: invalid rule")
	// ErrNotFound: no such Rule in this Workspace.
	ErrNotFound = errors.New("rules: rule not found")
	// ErrEventNotFound: no such Event in this Workspace (a dry run reads a stored Event).
	ErrEventNotFound = errors.New("rules: event not found")
)

// Action is one Action of a Rule as stored: its type and the parameters of that type.
type Action struct {
	Type string `json:"type"`
	// Tag is the Tag name of apply_tag and remove_tag.
	Tag string `json:"tag,omitempty"`
	// AutomationID is the Automation enroll_automation enrolls the Contact in.
	AutomationID int64 `json:"automation_id,omitempty"`

	// An http Action (ADR 0039 shape, shared through internal/httpcall) has ID, URL, Headers, Body
	// (a Liquid JSON template that sees the Contact and the `event`) and Secret. Stored, URL,
	// header values and Secret are ciphertext, sealed per field; in an Action being saved, an
	// empty one keeps the stored value of the Action of the same ID, and ClearSecret drops the
	// signing secret.
	ID          string            `json:"id,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     []httpcall.Header `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	Secret      string            `json:"secret,omitempty"`
	ClearSecret bool              `json:"-"`
}

// Outcome is how one Action ended.
type Outcome string

const (
	Succeeded Outcome = "succeeded"
	Failed    Outcome = "failed"
	// Skipped is an expected outcome (an Automation that is not active, a Contact already
	// enrolled), not an error.
	Skipped Outcome = "skipped"
)

// Result is what one Action did, as stored on its Firing.
type Result struct {
	Type   string  `json:"type"`
	Status Outcome `json:"status"`
	Detail string  `json:"detail,omitempty"`
}

// ActionInput is what a Runner is handed: the Action and the Firing it runs for.
type ActionInput struct {
	Action    Action
	ContactID int64
	Firing    *ent.RuleFiring
}

// Runner performs one kind of Action. It reports how it ended in the Result and never fails the
// others: an Action that cannot be done returns a Failed Result, and only an infrastructure
// error (the database) is returned as an error, which retries the Actions job.
type Runner interface {
	Run(ctx context.Context, s *ent.Scoped, in ActionInput) (Result, error)
}

// ActionRunners maps an Action type to the Runner that does it.
type ActionRunners map[string]Runner

// Module is the one place a Rule is written, validated, evaluated and run.
type Module struct {
	cipher *secrets.Cipher
	bus    *events.Bus
}

// New builds the Rules module. The cipher seals an http Action's secrets; the bus runs a Firing
// and the job that executes it in one transaction.
func New(cipher *secrets.Cipher, bus *events.Bus) *Module { return &Module{cipher: cipher, bus: bus} }

// CreateInput is a new Rule, always a draft.
type CreateInput struct {
	Name          string
	TriggerEvents []string
	// Condition is the encoded tree; nil or empty means every Contact.
	Condition *string
	Actions   []Action
}

// UpdateInput changes a Rule; nil fields are left as they are.
type UpdateInput struct {
	Name          *string
	TriggerEvents *[]string
	Condition     *string
	Actions       *[]Action
}

// Create validates and stores a draft Rule with its Trigger events.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Rule, error) {
	triggers, err := cleanTriggers(in.TriggerEvents)
	if err != nil {
		return nil, err
	}
	if err := m.validateCondition(ctx, s, in.Condition); err != nil {
		return nil, err
	}
	actions, err := m.encodeActions(in.Actions, nil)
	if err != nil {
		return nil, err
	}
	if err := requireAutomations(ctx, s, in.Actions); err != nil {
		return nil, err
	}
	var id int64
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		r, err := ts.Rule().Create().
			SetName(in.Name).
			SetNillableCondition(in.Condition).
			SetActions(actions).
			Save(ctx)
		if err != nil {
			return err
		}
		id = r.ID
		return saveTriggers(ctx, ts, id, triggers)
	})
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Update changes a Rule's name, Trigger events, Condition or Actions, validated as Create
// validates. The status is untouched.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Rule, error) {
	if err := m.validateCondition(ctx, s, in.Condition); err != nil {
		return nil, err
	}
	var triggers []string
	if in.TriggerEvents != nil {
		var err error
		if triggers, err = cleanTriggers(*in.TriggerEvents); err != nil {
			return nil, err
		}
	}
	var actions *string
	if in.Actions != nil {
		stored, err := m.Get(ctx, s, id)
		if err != nil {
			return nil, err
		}
		prev, err := DecodeActions(stored.Actions)
		if err != nil {
			return nil, err
		}
		encoded, err := m.encodeActions(*in.Actions, prev)
		if err != nil {
			return nil, err
		}
		if err := requireAutomations(ctx, s, *in.Actions); err != nil {
			return nil, err
		}
		actions = &encoded
	}
	err := m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		n, err := ts.Rule().Update().
			Where(rule.ID(id)).
			SetNillableName(in.Name).
			SetNillableCondition(in.Condition).
			SetNillableActions(actions).
			Save(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrNotFound
		}
		if in.TriggerEvents == nil {
			return nil
		}
		if _, err := ts.RuleTrigger().Delete().Where(ruletrigger.RuleID(id)).Exec(ctx); err != nil {
			return err
		}
		return saveTriggers(ctx, ts, id, triggers)
	})
	if err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Get returns one Rule of the Workspace with its Trigger events, archived or not.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Rule, error) {
	r, err := s.Rule().QueryWithDiscarded().Where(rule.ID(id)).WithTriggers(byAction).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNotFound
	}
	return r, err
}

// List returns one page of the Workspace's Rules, newest first: the kept ones, or with
// archived the archived ones.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Rule], error) {
	rows := func() *ent.RuleQuery {
		if archived {
			return s.Rule().QueryDiscarded()
		}
		return s.Rule().Query()
	}
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Rule, error) {
			return rows().WithTriggers(byAction).Order(ent.Desc(rule.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Archive discards a Rule (ADR 0049): it leaves the lists and acts on nothing, and its Firings
// stay. Archiving also deactivates it, so restoring it brings back a draft that cannot start
// firing before its author activates it again. Archiving an archived Rule changes nothing.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		// The write that deactivates also locks the row, so a concurrent Activate (which only
		// moves a kept draft) waits for the commit and then finds the Rule archived: no Rule is
		// ever archived and active. It matches no row for an archived or a missing Rule.
		n, err := ts.Rule().Update().
			Where(rule.ID(id), rule.DiscardedAtIsNil()).
			SetStatus(rule.StatusDraft).
			Save(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			_, err := m.Get(ctx, ts, id) // archived already: nothing to do; missing: ErrNotFound
			return err
		}
		return notFound(ts.Rule().Discard(ctx, id))
	})
}

// Unarchive brings an archived Rule back to the lists, as a draft. Restoring one that is not
// archived changes nothing.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) error {
	return notFound(s.Rule().Restore(ctx, id))
}

// Delete purges a Rule that has no Firings (ADR 0049). One with any Firing is history and is
// archived instead: an *inuse.Error carrying their count.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	n, err := s.Rule().Purge().Where(rule.ID(id), rule.Not(rule.HasFirings())).Exec(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	r, err := m.Get(ctx, s, id)
	if err != nil {
		return err
	}
	firings, err := s.RuleFiring().Query().Where(rulefiring.RuleID(id)).Count(ctx)
	if err != nil {
		return err
	}
	return &inuse.Error{What: inuse.SubjectRule, Users: []inuse.User{inuse.Firings(id, r.Name, firings)}}
}

// UsingTag returns the Workspace's active Rules, in id order, that name the Tag by its name: in
// a Condition leaf or in a tag Action. A draft or an archived Rule acts on nothing, so it does
// not hold the Tag.
func UsingTag(ctx context.Context, s *ent.Scoped, name string) ([]*ent.Rule, error) {
	active, err := s.Rule().Query().Where(rule.StatusEQ(rule.StatusActive)).Order(ent.Asc(rule.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	return lo.Filter(active, func(r *ent.Rule, _ int) bool {
		if g, err := condition.Parse(lo.FromPtr(r.Condition)); err == nil && g.NamesTag(name) {
			return true
		}
		actions, err := DecodeActions(r.Actions)
		return err == nil && slices.ContainsFunc(actions, func(a Action) bool {
			return (a.Type == ActionApplyTag || a.Type == ActionRemoveTag) && a.Tag == name
		})
	}), nil
}

func notFound(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Activate makes a Rule react to the Events recorded from now on (it never replays history).
// Activating an active Rule leaves it as it is.
// A Rule with no Actions does nothing, so it is refused (ErrInvalid).
func (m *Module) Activate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Rule, error) {
	r, err := m.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	if r.DiscardedAt != nil {
		return nil, ErrNotFound // an archived Rule acts on nothing: restore it first
	}
	actions, err := DecodeActions(r.Actions)
	if err != nil {
		return nil, err
	}
	if len(actions) == 0 {
		return nil, fmt.Errorf("%w: a Rule needs at least one Action to be activated", ErrInvalid)
	}
	if _, err := s.Rule().Update().
		Where(rule.ID(id), rule.DiscardedAtIsNil(), rule.StatusEQ(rule.StatusDraft)).
		SetStatus(rule.StatusActive).
		SetActivatedAt(time.Now()).
		Save(ctx); err != nil {
		return nil, err
	}
	return m.Get(ctx, s, id)
}

// Deactivate stops a Rule from firing. Deactivating a draft leaves it as it is.
func (m *Module) Deactivate(ctx context.Context, s *ent.Scoped, id int64) (*ent.Rule, error) {
	n, err := s.Rule().Update().
		Where(rule.ID(id), rule.DiscardedAtIsNil()).
		SetStatus(rule.StatusDraft).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return m.Get(ctx, s, id)
}

// Firings returns one page of a Rule's Firings, newest first.
func (m *Module) Firings(ctx context.Context, s *ent.Scoped, ruleID int64, p pagination.Params) (pagination.Page[*ent.RuleFiring], error) {
	if _, err := m.Get(ctx, s, ruleID); err != nil {
		return pagination.Page[*ent.RuleFiring]{}, err
	}
	rows := func() *ent.RuleFiringQuery { return s.RuleFiring().Query().Where(rulefiring.RuleID(ruleID)) }
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.RuleFiring, error) {
			return rows().Order(ent.Desc(rulefiring.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}

// Triggers returns a Rule's Trigger event actions, in order.
func Triggers(r *ent.Rule) []string {
	return lo.Map(r.Edges.Triggers, func(t *ent.RuleTrigger, _ int) string { return t.Action })
}

// byAction orders a Rule's Trigger events so a read is stable.
func byAction(q *ent.RuleTriggerQuery) { q.Order(ent.Asc(ruletrigger.FieldID)) }

// DecodeActions parses a stored Actions array.
func DecodeActions(stored string) ([]Action, error) {
	if stored == "" {
		return nil, nil
	}
	var actions []Action
	if err := json.Unmarshal([]byte(stored), &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

// DecodeResults parses a Firing's stored results.
func DecodeResults(stored string) ([]Result, error) {
	if stored == "" {
		return nil, nil
	}
	var results []Result
	if err := json.Unmarshal([]byte(stored), &results); err != nil {
		return nil, err
	}
	return results, nil
}

func cleanTriggers(in []string) ([]string, error) {
	out := lo.Uniq(lo.FilterMap(in, func(a string, _ int) (string, bool) {
		a = strings.TrimSpace(a)
		return a, a != ""
	}))
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: a Rule needs at least one Trigger event", ErrInvalid)
	}
	return out, nil
}

func saveTriggers(ctx context.Context, s *ent.Scoped, ruleID int64, actions []string) error {
	creates := lo.Map(actions, func(a string, _ int) *ent.RuleTriggerScopedCreate {
		return s.RuleTrigger().Create().SetRuleID(ruleID).SetAction(a)
	})
	return s.RuleTrigger().CreateBulk(creates...).Exec(ctx)
}

// validateCondition checks a Condition as a Segment's is checked: it parses, and every leaf names
// a Contact attribute, a Tag, an Event action or a custom field the Workspace has. A Rule's
// Condition may also test the triggering Event's properties ("trigger:<key>", any key).
func (m *Module) validateCondition(ctx context.Context, s *ent.Scoped, cond *string) error {
	if cond == nil || *cond == "" {
		return nil
	}
	if g, err := condition.Parse(*cond); err == nil && g.Size() > MaxConditionNodes {
		return fmt.Errorf("%w: a Condition has at most %d groups and rules, this one has %d", ErrInvalid, MaxConditionNodes, g.Size())
	}
	if _, err := condition.RulePredicate(ctx, s, *cond, nil); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	return nil
}

// encodeActions validates the Actions and returns the stored JSON, dropping the fields that do
// not belong to each type. stored are the Rule's Actions as saved now (none for a new Rule): an
// http Action that names one of them by ID keeps the secrets it does not resend.
func (m *Module) encodeActions(actions, stored []Action) (string, error) {
	if len(actions) > MaxActions {
		return "", fmt.Errorf("%w: a Rule runs at most %d Actions, this one has %d", ErrInvalid, MaxActions, len(actions))
	}
	prev := map[string]Action{}
	for i := range stored {
		p := &stored[i]
		if p.Type == ActionHTTP {
			prev[p.ID] = *p
		}
	}
	out := make([]Action, 0, len(actions))
	seen := map[string]bool{}
	for i := range actions {
		a := &actions[i]
		switch a.Type {
		case ActionApplyTag, ActionRemoveTag:
			tag := strings.TrimSpace(a.Tag)
			if tag == "" {
				return "", fmt.Errorf("%w: action %d: a tag action needs a tag", ErrInvalid, i+1)
			}
			out = append(out, Action{Type: a.Type, Tag: tag})
		case ActionEnroll:
			if a.AutomationID <= 0 {
				return "", fmt.Errorf("%w: action %d: an enroll action needs an automation", ErrInvalid, i+1)
			}
			out = append(out, Action{Type: a.Type, AutomationID: a.AutomationID})
		case ActionHTTP:
			if len(seen) >= MaxHTTPActionsPerRule {
				return "", fmt.Errorf("%w: a Rule has at most %d http Actions", ErrInvalid, MaxHTTPActionsPerRule)
			}
			sealed, err := m.sealHTTP(*a, prev)
			if bad, ok := errors.AsType[httpcall.InvalidError](err); ok {
				return "", fmt.Errorf("%w: action %d: %w", ErrInvalid, i+1, bad)
			}
			if err != nil {
				return "", err
			}
			if seen[sealed.ID] {
				return "", fmt.Errorf("%w: action %d: duplicate http Action id", ErrInvalid, i+1)
			}
			seen[sealed.ID] = true
			out = append(out, sealed)
		default:
			return "", fmt.Errorf("%w: action %d: unknown action type %q", ErrInvalid, i+1, a.Type)
		}
	}
	b, err := json.Marshal(out)
	return string(b), err
}

// sealHTTP turns an http Action being saved into its stored form. An Action that names a stored
// one by ID keeps what it does not resend; any other gets a new ID and no stored values.
func (m *Module) sealHTTP(a Action, prev map[string]Action) (Action, error) {
	p, known := prev[a.ID]
	out := Action{Type: ActionHTTP, ID: a.ID}
	if !known || a.ID == "" {
		p, out.ID = Action{}, ulid.Make().String()
	}
	sealed, err := httpcall.Seal(m.cipher, httpSpec(a), httpSpec(p), "http action")
	if err != nil {
		return out, err
	}
	out.URL, out.Headers, out.Body, out.Secret = sealed.URL, sealed.Headers, sealed.Body, sealed.Secret
	return out, nil
}

func httpSpec(a Action) httpcall.Spec {
	return httpcall.Spec{URL: a.URL, Headers: a.Headers, Body: a.Body, Secret: a.Secret, ClearSecret: a.ClearSecret}
}

// OpenHTTP decrypts a stored http Action's URL, header values and secret, for its Runner.
func (m *Module) OpenHTTP(a Action) (httpcall.Opened, error) {
	return httpcall.Open(m.cipher, httpSpec(a))
}

// Event is what Evaluate needs of a recorded Event.
type Event struct {
	ContactID int64
	// Action is the Event's action, matched against the Rules' Trigger events.
	Action string
	// SourceID identifies the Event: the DedupKey, else the envelope's id (what persist stores
	// as the Event's source_id). A Rule fires once per SourceID.
	SourceID string
	// RecordedAt is when the Event was recorded (published to the bus, not when the client says
	// it happened); a Rule acts only on Events recorded at or after its activation.
	RecordedAt time.Time
	// Properties are the properties the Event carries.
	Properties map[string]any
}

// ActionsArgs is the job that runs a Firing's Actions. It carries no personal data: the Firing
// is read from the database, and goes with its Contact on Erasure.
type ActionsArgs struct {
	FiringID int64 `json:"firing_id"`
}

// ActionsKind is the river kind of ActionsArgs.
const ActionsKind = "rule_run_actions"

func (ActionsArgs) Kind() string { return ActionsKind }

// Evaluate matches an Event against the Workspace's active Rules (ADR 0054). For each Rule that
// has the Event's action as a Trigger event, was activated before the Event and whose Condition
// holds for the Contact right now, it writes a Firing and queues its Actions job in one
// transaction. The Firing is unique per Rule and SourceID, so an Event delivered twice fires a
// Rule once. A Rule whose Condition is false leaves nothing. Rules for one Event are independent:
// a failure on one is returned after the others have been tried.
//
// It returns the Firings it wrote. It is queue-free apart from the enqueue inside the
// transaction, so a test drives it directly.
func (m *Module) Evaluate(ctx context.Context, s *ent.Scoped, ev Event) ([]*ent.RuleFiring, error) {
	ev.Properties = events.RuleProperties(ev.Properties)
	active, err := s.Rule().Query().
		Where(
			rule.StatusEQ(rule.StatusActive),
			rule.ActivatedAtLTE(ev.RecordedAt),
			triggeredBy(ev.Action),
		).
		Order(ent.Asc(rule.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var fired []*ent.RuleFiring
	var failures []error
	for _, r := range active {
		holds, err := m.holds(ctx, s, r, ev)
		if err != nil {
			failures = append(failures, fmt.Errorf("rule %d: %w", r.ID, err))
			continue
		}
		if !holds {
			continue
		}
		f, err := m.fire(ctx, s, r, ev)
		if err != nil {
			failures = append(failures, fmt.Errorf("rule %d: %w", r.ID, err))
			continue
		}
		if f != nil {
			fired = append(fired, f)
		}
	}
	return fired, errors.Join(failures...)
}

// triggeredBy selects the Rules that have the action as a Trigger event: the matching step a real
// firing (Evaluate) and a dry run (DryRun) share.
func triggeredBy(action string) predicate.Rule {
	return rule.HasTriggersWith(ruletrigger.Action(action))
}

// holds evaluates the Rule's Condition for the Contact against its current data. A Condition
// that no longer compiles (a custom field it named has since been deleted) holds for nobody:
// retrying cannot fix it, and the other Rules must still run.
func (m *Module) holds(ctx context.Context, s *ent.Scoped, r *ent.Rule, ev Event) (bool, error) {
	match := contact.ID(ev.ContactID)
	if r.Condition != nil && *r.Condition != "" {
		pred, err := condition.RulePredicate(ctx, s, *r.Condition, ev.Properties)
		if errors.Is(err, condition.ErrStorage) {
			return false, err
		}
		if err != nil {
			return false, nil
		}
		match = contact.And(match, pred)
	}
	return s.Contact().Query().Where(match).Exist(ctx)
}

// fire writes the Rule's Firing for the Event and queues its Actions job in the same
// transaction. It returns nil when the Rule already fired for this Event.
func (m *Module) fire(ctx context.Context, s *ent.Scoped, r *ent.Rule, ev Event) (*ent.RuleFiring, error) {
	var fired *ent.RuleFiring
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, q events.Enqueuer) error {
		// Check first so a redelivered Event does not trip the unique index (a violation would
		// poison the transaction); the index stays as the race safety net.
		exists, err := ts.RuleFiring().Query().
			Where(rulefiring.RuleID(r.ID), rulefiring.SourceID(ev.SourceID)).
			Exist(ctx)
		if err != nil || exists {
			return err
		}
		f, err := ts.RuleFiring().Create().
			SetRuleID(r.ID).
			SetContactID(ev.ContactID).
			SetAction(ev.Action).
			SetSourceID(ev.SourceID).
			SetActions(r.Actions).
			SetNillableProperties(propertiesJSON(ev.Properties)).
			Save(ctx)
		if err != nil {
			return err
		}
		fired = f
		return q.Enqueue(ctx, ActionsArgs{FiringID: f.ID}, &river.InsertOpts{})
	})
	if ent.IsConstraintError(err) {
		return nil, nil // lost a race with the same Event delivered twice
	}
	return fired, err
}

// RunActions runs a Firing's Actions in order, each independent of the others: one that fails
// is recorded as failed and the next still runs. Each result is stored as soon as it is known,
// so a retry of the job (after a database error) resumes after the last recorded Action instead
// of repeating it. A Firing that is gone (erased with its Contact) or finished does nothing.
func (m *Module) RunActions(ctx context.Context, s *ent.Scoped, firingID int64, runners ActionRunners) error {
	f, err := s.RuleFiring().Get(ctx, firingID)
	if ent.IsNotFound(err) || (err == nil && f.CompletedAt != nil) {
		return nil
	}
	if err != nil {
		return err
	}
	actions, err := DecodeActions(f.Actions)
	if err != nil {
		return fmt.Errorf("firing %d actions: %w", f.ID, err)
	}
	results, err := DecodeResults(f.Results)
	if err != nil {
		return fmt.Errorf("firing %d results: %w", f.ID, err)
	}
	pending := actions[min(len(results), len(actions)):]
	for i := range pending {
		a := &pending[i]
		res := Result{Type: a.Type, Status: Failed, Detail: "no runner for this action type"}
		if runner, ok := runners[a.Type]; ok {
			if res, err = runner.Run(ctx, s, ActionInput{Action: *a, ContactID: f.ContactID, Firing: f}); err != nil {
				return err
			}
			res.Type = a.Type
		}
		results = append(results, res)
		encoded, err := json.Marshal(results)
		if err != nil {
			return err
		}
		if err := s.RuleFiring().UpdateOneID(f.ID).SetResults(string(encoded)).Exec(ctx); err != nil {
			return err
		}
	}
	return s.RuleFiring().UpdateOneID(f.ID).SetCompletedAt(time.Now()).Exec(ctx)
}

// requireAutomations refuses an enroll Action whose Automation is not a kept Automation of the
// Workspace (ErrInvalid).
func requireAutomations(ctx context.Context, s *ent.Scoped, actions []Action) error {
	for i := range actions {
		a := &actions[i]
		if a.Type != ActionEnroll {
			continue
		}
		ok, err := s.Automation().Query().Where(automation.ID(a.AutomationID)).Exist(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%w: action %d: automation %d not found", ErrInvalid, i+1, a.AutomationID)
		}
	}
	return nil
}

// EnrollingInto returns the active Rules that have an Action enrolling Contacts in the Automation.
// A removal of the Automation is refused while there are any (ADR 0049, ADR 0054).
func EnrollingInto(ctx context.Context, s *ent.Scoped, automationID int64) ([]*ent.Rule, error) {
	active, err := s.Rule().Query().Where(rule.StatusEQ(rule.StatusActive)).Order(ent.Asc(rule.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	var using []*ent.Rule
	for _, r := range active {
		actions, err := DecodeActions(r.Actions)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(actions, func(a Action) bool {
			return a.Type == ActionEnroll && a.AutomationID == automationID
		}) {
			using = append(using, r)
		}
	}
	return using, nil
}

// propertiesJSON is the Event's properties as kept on its Firing, nil when there are none. The
// caller has already bounded them (events.RuleProperties), so a Firing keeps exactly what its
// Rule's Condition saw.
func propertiesJSON(props map[string]any) *string {
	if len(props) == 0 {
		return nil
	}
	b, err := json.Marshal(props)
	if err != nil {
		return nil
	}
	return new(string(b))
}
