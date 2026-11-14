// Package needsattentionreport is the use case of the Needs attention report (GLOSSARY, ADR 0102),
// which reads what in a Workspace Needs attention: what needs a member's action now, what has stopped its mail, what slows it or puts it at risk,
// and what failed without a retry. It is the overview's first block, shown only while it holds
// an item, so every item is true at the moment it is read and none is stored: the facts already
// live on the Sending domains, Integrations, Broadcasts, Automation runs and Webhook endpoints,
// and this package only gathers them. Whether an Automation can send is not stored anywhere: the
// send path's Preflight answers it.
//
// A Workspace suspension and a Staff hold are not items: the layout shows them on every page.
package needsattentionreport

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/domain/insights/abusewarning"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// Kind is what an item is about.
type Kind string

const (
	// KindSendingDomainVerificationLost: a re-check found a verified Sending domain's DKIM record
	// gone, and its mail is held.
	KindSendingDomainVerificationLost Kind = "sendingDomainVerificationLost"
	// KindIntegrationProviderStopped: the provider has stopped the Integration's account, and
	// every send through it is held (ADR 0067).
	KindIntegrationProviderStopped Kind = "integrationProviderStopped"
	// KindIntegrationProviderRefused: a send found the provider refusing the Integration's
	// credentials or account, and every send through it is held until it is saved again.
	KindIntegrationProviderRefused Kind = "integrationProviderRefused"
	// KindBroadcastHeld: a Broadcast whose sending is held for a reason a member can fix.
	KindBroadcastHeld Kind = "broadcastHeld"
	// KindAutomationCannotSend: an active Automation whose live version has an email step that a
	// send would hold now, for a reason of its own source.
	KindAutomationCannotSend Kind = "automationCannotSend"
	// KindDeliverabilityWarning: a Sending domain is in the abuse detector's warning tier.
	KindDeliverabilityWarning Kind = "deliverabilityWarning"
	// KindSendCapSpent: the Workspace sent its Send cap over the last 24 hours, and sending is
	// deferred until it refills.
	KindSendCapSpent Kind = "sendCapSpent"
	// KindScheduledBroadcastOverSendCap: a Broadcast scheduled within the next 24 hours that the
	// Workspace's Send cap will not let out in full when it starts, together with the Broadcasts
	// scheduled before it, so it is deferred (ADR 0067).
	KindScheduledBroadcastOverSendCap Kind = "scheduledBroadcastOverSendCap"
	// KindProviderLimitSpent: an Integration sent its Provider limit over the last 24 hours.
	KindProviderLimitSpent Kind = "providerLimitSpent"
	// KindIntegrationProviderProbation: the provider has the Integration's account under review.
	KindIntegrationProviderProbation Kind = "integrationProviderProbation"
	// KindIntegrationProviderSandbox: the provider's sandbox sends only to verified addresses.
	KindIntegrationProviderSandbox Kind = "integrationProviderSandbox"
	// KindEnrollmentsFailed: runs of an Automation failed in the last week.
	KindEnrollmentsFailed Kind = "enrollmentsFailed"
	// KindBroadcastFailed: a Broadcast failed in the last week.
	KindBroadcastFailed Kind = "broadcastFailed"
	// KindWebhookDisabled: a Webhook endpoint disabled itself after failing deliveries.
	KindWebhookDisabled Kind = "webhookDisabled"
	// KindSendingDomainNotBulkReady: a verified Sending domain's last check found no published
	// DMARC record, which bulk mail needs (ADR 0012); it still sends.
	KindSendingDomainNotBulkReady Kind = "sendingDomainNotBulkReady"
)

// Severity orders the items: what stops mail, then what slows it or puts it at risk, then what
// failed and was not retried or what bulk mail needs set up.
type Severity string

const (
	SeverityBlocked  Severity = "blocked"
	SeverityDegraded Severity = "degraded"
	SeverityNotice   Severity = "notice"
)

// severities is the order of the list, most urgent first.
var severities = []Severity{SeverityBlocked, SeverityDegraded, SeverityNotice}

// severity is how urgent an item of the Kind is: its Kind alone decides it.
func (k Kind) severity() Severity {
	switch k {
	case KindSendingDomainVerificationLost, KindIntegrationProviderStopped, KindIntegrationProviderRefused,
		KindBroadcastHeld, KindAutomationCannotSend:
		return SeverityBlocked
	case KindDeliverabilityWarning, KindSendCapSpent, KindScheduledBroadcastOverSendCap, KindProviderLimitSpent,
		KindIntegrationProviderProbation, KindIntegrationProviderSandbox:
		return SeverityDegraded
	case KindEnrollmentsFailed, KindBroadcastFailed, KindWebhookDisabled, KindSendingDomainNotBulkReady:
		return SeverityNotice
	}
	panic("needsattentionreport: no severity for kind " + string(k))
}

// recent is how far back a failure that is not retried stays an item.
const recent = 7 * 24 * time.Hour

// Subject is the object an item is about.
type Subject struct {
	ID   int64
	Name string
}

// Item is one thing that needs a member's action.
type Item struct {
	Kind     Kind
	Severity Severity
	// Subject is the object the item is about; nil for the Workspace's own Send cap.
	Subject *Subject
	// Count is how many the item stands for: failed runs of an Automation, the estimated audience
	// of a Broadcast over the Send cap, 1 for the rest.
	Count int
	// Since is when the problem began, when that is known.
	Since *time.Time
	// HoldReason is why a held Broadcast is held (the Broadcast's own hold reason), or what a send
	// of an Automation that cannot send would be held for now.
	HoldReason *holdreason.Reason
	// Abuse is the rate behind a deliverability warning.
	Abuse *abusewarning.Warning
	// EstimatedCompletionAt is when the Send cap is expected to have let a Broadcast over it out.
	EstimatedCompletionAt *time.Time
}

// UseCase reads the items.
type UseCase struct {
	now   func() time.Time
	watch abusewarning.Watch
	out   *outboundmessages.Module
}

// New is the use case: watch is the abuse detector's warning tier, nil where the detector does not
// run, out is the send path, whose Preflight says whether an Automation's email step could send
// now and whose SendCapStatus says which Send cap applies (ADR 0067), and now is the Instance clock.
func New(watch abusewarning.Watch, out *outboundmessages.Module, now func() time.Time) *UseCase {
	return &UseCase{now: now, watch: watch, out: out}
}

// Items is what needs attention in the Workspace behind s, most urgent first and, within a
// severity, the oldest first.
func (m *UseCase) Items(ctx context.Context, s *ent.Scoped) ([]Item, error) {
	now := m.now()
	readers := []func(context.Context, *ent.Scoped, time.Time) ([]Item, error){
		domainsLost,
		m.integrations,
		heldBroadcasts,
		m.automationsCannotSend,
		m.deliverability,
		m.sendCap,
		failedRuns,
		failedBroadcasts,
		disabledWebhooks,
		domainsNotBulkReady,
	}
	var items []Item
	for _, read := range readers {
		got, err := read(ctx, s, now)
		if err != nil {
			return nil, err
		}
		items = append(items, got...)
	}
	for i := range items {
		items[i].Severity = items[i].Kind.severity()
	}
	slices.SortStableFunc(items, func(a, b Item) int {
		return cmp.Or(
			cmp.Compare(slices.Index(severities, a.Severity), slices.Index(severities, b.Severity)),
			compareSince(a.Since, b.Since),
		)
	})
	return items, nil
}

// compareSince puts the older problem first and one with no start last.
func compareSince(a, b *time.Time) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	return a.Compare(*b)
}

func domainsLost(ctx context.Context, s *ent.Scoped, _ time.Time) ([]Item, error) {
	domains, err := s.SendingDomain().Query().
		Where(sendingdomain.VerificationLostAtNotNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(domains))
	for _, d := range domains {
		items = append(items, Item{
			Kind: KindSendingDomainVerificationLost, Count: 1,
			Subject: &Subject{ID: d.ID, Name: d.Domain}, Since: d.VerificationLostAt,
		})
	}
	return items, nil
}

// integrations reads the standing of every enabled Integration as the provider last reported
// it, and whether it sent its Provider limit over the last 24 hours.
func (m *UseCase) integrations(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	integs, err := s.Integration().Query().Where(integration.Enabled(true)).All(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := providerlimit.Usage(ctx, s, now)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, integ := range integs {
		subject := &Subject{ID: integ.ID, Name: integ.Name}
		eff := providerlimit.EffectiveOf(integ)
		if integ.ProviderRefusedAt != nil {
			items = append(items, Item{Kind: KindIntegrationProviderRefused, Count: 1, Subject: subject, Since: integ.ProviderRefusedAt})
		}
		switch eff.Enforcement {
		case providers.EnforcementShutdown:
			items = append(items, Item{Kind: KindIntegrationProviderStopped, Count: 1, Subject: subject})
		case providers.EnforcementProbation:
			items = append(items, Item{Kind: KindIntegrationProviderProbation, Count: 1, Subject: subject})
		case providers.EnforcementNone:
		}
		if eff.Sandbox {
			items = append(items, Item{Kind: KindIntegrationProviderSandbox, Count: 1, Subject: subject})
		}
		if limit := eff.PerDay.Limit; limit != nil && usage[integ.ID] >= *limit {
			items = append(items, Item{Kind: KindProviderLimitSpent, Count: 1, Subject: subject})
		}
	}
	return items, nil
}

// heldBroadcasts reads the Broadcasts still to send whose sending is held by a check of their own
// source; a failed Broadcast keeps the reason it was held for, and is a failure, not a hold. A
// Workspace-level hold (a suspension, a Staff hold, a pending deletion) is left out: the layout
// shows it on every page.
func heldBroadcasts(ctx context.Context, s *ent.Scoped, _ time.Time) ([]Item, error) {
	held, err := s.Broadcast().Query().
		Where(
			broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending),
			broadcast.HoldReasonIn(sourceHolds...),
		).
		Order(ent.Asc(broadcast.FieldID)).
		Select(broadcast.FieldID, broadcast.FieldName, broadcast.FieldHoldReason).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(held))
	for _, b := range held {
		items = append(items, Item{
			Kind: KindBroadcastHeld, Count: 1,
			Subject: &Subject{ID: b.ID, Name: b.Name}, HoldReason: b.HoldReason,
		})
	}
	return items, nil
}

// sourceHolds are the holds of an object's own source that make an item; a Workspace-level hold
// (a suspension, a Staff hold, a pending deletion) is left out: the layout shows it on every page.
var sourceHolds = holdreason.Source()

// automationsCannotSend reads the active Automations whose live version has a send step that a
// send would hold now for a reason of its own source: whatever Send would hold, the item shows,
// once per Automation, with the reason of its first such step. A send step has no From of its own,
// so the answer depends only on the step's Channel and the Integration it names (or that Channel's
// default), and the send path is asked once per such pair, not once per step. Nothing stores when
// a route began to hold, so the item has no since.
func (m *UseCase) automationsCannotSend(ctx context.Context, s *ent.Scoped, _ time.Time) ([]Item, error) {
	senders, err := automations.Senders(ctx, s)
	if err != nil {
		return nil, err
	}
	// The answer of each Channel and Integration choice; the default is under 0, which no row has.
	type choice struct {
		ch          channels.Channel
		integration int64
	}
	holdOf := map[choice]holdreason.Reason{}
	var items []Item
	for _, a := range senders {
		steps, err := automations.SendSteps(a.Edges.LiveVersion)
		if err != nil {
			return nil, err
		}
		for i := range steps {
			st := &steps[i]
			key := choice{st.Channel(), lo.FromPtr(st.Step.IntegrationID)}
			hold, asked := holdOf[key]
			if !asked {
				content, err := st.Content()
				if err != nil {
					return nil, err
				}
				if hold, err = m.out.Preflight(ctx, s, content, st.Step.IntegrationID); err != nil {
					return nil, err
				}
				holdOf[key] = hold
			}
			if !slices.Contains(sourceHolds, hold) {
				continue
			}
			items = append(items, Item{
				Kind: KindAutomationCannotSend, Count: 1,
				Subject: &Subject{ID: a.ID, Name: a.Name}, HoldReason: &hold,
			})
			break
		}
	}
	return items, nil
}

func (m *UseCase) deliverability(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	if m.watch == nil {
		return nil, nil
	}
	w, err := m.watch.Warning(ctx, s, now)
	if err != nil || w == nil {
		return nil, err
	}
	return []Item{{Kind: KindDeliverabilityWarning, Count: 1, Abuse: w}}, nil
}

// sendCap reads the Send cap that applies to the Workspace, as the send path enforces it: spent
// over the last 24 hours, and the scheduled Broadcasts it will not let out in full. Nothing
// applies where no Send cap does.
func (m *UseCase) sendCap(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	st, err := m.out.SendCapStatus(ctx, s, ws)
	if err != nil || st == nil {
		return nil, err
	}
	var items []Item
	if st.Sent24h >= st.PerDay {
		items = append(items, Item{Kind: KindSendCapSpent, Count: 1})
	}
	over, err := scheduledOverSendCap(ctx, s, st.PerDay, now)
	if err != nil {
		return nil, err
	}
	return append(items, over...), nil
}

// overCapWindow is how far ahead a scheduled Broadcast is judged against the Send cap; one further
// ahead is judged by a later read.
const overCapWindow = 24 * time.Hour

// scheduledOverSendCap reads the scheduled Broadcasts, held ones aside, that start within the next
// 24 hours and that the Workspace's Send cap will not let out in full when they start. Each is
// judged by the bucket as it will be at its start (now, for one past due that has not started
// yet), the bucket the send path paces by: the wait is how long it then takes to let out the
// Broadcast's estimated audience, those of every one starting before it and the recipients the
// sending Broadcasts have left, since they all draw from the same bucket, and the Broadcast is an
// item when there is a wait, finishing at its start plus the wait. Taking the earlier audiences at
// its start leaves out what the bucket refills between their starts, so the read errs towards an
// item. perDay is the Send cap that applies.
func scheduledOverSendCap(ctx context.Context, s *ent.Scoped, perDay int, now time.Time) ([]Item, error) {
	scheduled, err := s.Broadcast().Query().
		Where(
			broadcast.StatusEQ(broadcast.StatusScheduled),
			broadcast.HoldReasonIsNil(),
			broadcast.ScheduledAtLTE(now.Add(overCapWindow)),
		).
		Order(ent.Asc(broadcast.FieldScheduledAt), ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	// What the sending Broadcasts still have to send goes out of the bucket before any of these.
	accumulated, err := broadcasts.PendingOfSending(ctx, s)
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, b := range scheduled {
		audience, err := broadcasts.AudienceSize(ctx, s, b, now)
		if errors.Is(err, broadcasts.ErrNoAudience) {
			// Its plan fails it instead, a failure of its own: it draws nothing from the bucket.
			continue
		}
		if err != nil {
			return nil, err
		}
		accumulated += audience
		start := *b.ScheduledAt
		if start.Before(now) {
			start = now
		}
		wait, err := providerlimit.SendCapDrain(ctx, s, perDay, accumulated, start)
		if err != nil {
			return nil, err
		}
		if wait == 0 {
			continue
		}
		finish := start.Add(wait)
		items = append(items, Item{
			Kind: KindScheduledBroadcastOverSendCap, Count: audience,
			Subject: &Subject{ID: b.ID, Name: b.Name}, EstimatedCompletionAt: &finish,
		})
	}
	return items, nil
}

// failedRuns reads, per kept Automation, the runs that failed in the last week, counted and dated
// by the first of them; an archived Automation asks for no attention (ADR 0099), as an archived
// Broadcast does not.
func failedRuns(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	var groups []struct {
		AutomationID int64     `json:"automation_id"`
		Count        int       `json:"count"`
		Since        time.Time `json:"min"`
	}
	err := s.AutomationRun().Query().
		Where(
			automationrun.StatusEQ(automationrun.StatusFailed),
			automationrun.EndedAtGTE(now.Add(-recent)),
			automationrun.HasAutomationWith(automation.ArchivedAtIsNil()),
		).
		GroupBy(automationrun.FieldAutomationID).
		Aggregate(ent.Count(), ent.Min(automationrun.FieldEndedAt)).
		Scan(ctx, &groups)
	if err != nil || len(groups) == 0 {
		return nil, err
	}
	ids := make([]int64, len(groups))
	for i, g := range groups {
		ids[i] = g.AutomationID
	}
	kept, err := s.Automation().Query().
		Where(automation.IDIn(ids...)).
		Select(automation.FieldID, automation.FieldName).
		All(ctx)
	if err != nil {
		return nil, err
	}
	nameOf := make(map[int64]string, len(kept))
	for _, a := range kept {
		nameOf[a.ID] = a.Name
	}
	items := make([]Item, 0, len(groups))
	for _, g := range groups {
		name, ok := nameOf[g.AutomationID]
		if !ok {
			continue
		}
		since := g.Since
		items = append(items, Item{
			Kind: KindEnrollmentsFailed, Count: g.Count,
			Subject: &Subject{ID: g.AutomationID, Name: name}, Since: &since,
		})
	}
	return items, nil
}

func failedBroadcasts(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	failed, err := s.Broadcast().Query().
		Where(broadcast.StatusEQ(broadcast.StatusFailed), broadcast.FailedAtGTE(now.Add(-recent))).
		Select(broadcast.FieldID, broadcast.FieldName, broadcast.FieldFailedAt).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(failed))
	for _, b := range failed {
		items = append(items, Item{
			Kind: KindBroadcastFailed, Count: 1,
			Subject: &Subject{ID: b.ID, Name: b.Name}, Since: b.FailedAt,
		})
	}
	return items, nil
}

// domainsNotBulkReady reads the verified, kept Sending domains whose last DMARC lookup that
// answered found no record. A domain no DMARC lookup has answered for is not judged (its DKIM
// check says nothing of DMARC), and an unverified one is the loss item's or not yet sending.
func domainsNotBulkReady(ctx context.Context, s *ent.Scoped, _ time.Time) ([]Item, error) {
	domains, err := s.SendingDomain().Query().
		Where(dkimstate.Verified(), sendingdomain.DmarcCheckedAtNotNil(), sendingdomain.DmarcFoundAtIsNil()).
		Order(ent.Asc(sendingdomain.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(domains))
	for _, d := range domains {
		items = append(items, Item{
			Kind: KindSendingDomainNotBulkReady, Count: 1,
			Subject: &Subject{ID: d.ID, Name: d.Domain},
		})
	}
	return items, nil
}

func disabledWebhooks(ctx context.Context, s *ent.Scoped, _ time.Time) ([]Item, error) {
	hooks, err := s.WebhookEndpoint().Query().Where(webhookendpoint.DisabledAtNotNil()).All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(hooks))
	for _, h := range hooks {
		items = append(items, Item{
			Kind: KindWebhookDisabled, Count: 1,
			Subject: &Subject{ID: h.ID, Name: h.URL}, Since: h.DisabledAt,
		})
	}
	return items, nil
}
