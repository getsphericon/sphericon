// Package attention reads what in a Workspace needs a member's action now: what has stopped its
// mail, what slows it or puts it at risk, and what failed without a retry. It is the overview's
// first block, shown only while it holds an item, so every item is true at the moment it is read
// and none is stored: the facts already live on the Sending domains, Integrations, Broadcasts,
// Automation runs and Webhook endpoints, and this package only gathers them.
//
// A Workspace suspension and a Staff hold are not items: the layout shows them on every page.
package attention

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/abusewatch"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/providerlimit"
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
	// KindBroadcastHeld: a Broadcast whose sending is held for a reason a member can fix.
	KindBroadcastHeld Kind = "broadcastHeld"
	// KindDeliverabilityWarning: a Sending domain is in the abuse detector's warning tier.
	KindDeliverabilityWarning Kind = "deliverabilityWarning"
	// KindSendCapSpent: the Workspace sent its Send cap over the last 24 hours, and sending is
	// deferred until it refills.
	KindSendCapSpent Kind = "sendCapSpent"
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
)

// Severity orders the items: what stops mail, then what slows it or puts it at risk, then what
// failed and was not retried.
type Severity string

const (
	SeverityBlocked  Severity = "blocked"
	SeverityDegraded Severity = "degraded"
	SeverityNotice   Severity = "notice"
)

// severities is the order of the list, most urgent first.
var severities = []Severity{SeverityBlocked, SeverityDegraded, SeverityNotice}

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
	// Count is how many failures the item stands for: failed runs of an Automation, 1 for the rest.
	Count int
	// Since is when the problem began, when that is known.
	Since *time.Time
	// HoldReason is why a held Broadcast is held (the Broadcast's own hold reason).
	HoldReason *string
	// Abuse is the rate behind a deliverability warning.
	Abuse *abusewatch.Warning
}

// Module reads the items. Its clock is time.Now.
type Module struct {
	now      func() time.Time
	watch    abusewatch.Watch
	sendCaps providerlimit.SendCaps
}

// New is the module: watch is the abuse detector's warning tier, nil where the detector does not
// run, and sendCaps says whether the Instance has Send caps (ADR 0067).
func New(watch abusewatch.Watch, sendCaps providerlimit.SendCaps) *Module {
	return &Module{now: time.Now, watch: watch, sendCaps: sendCaps}
}

// WithClock returns the module reading now from the given clock.
func (m *Module) WithClock(now func() time.Time) *Module {
	c := *m
	c.now = now
	return &c
}

// Items is what needs attention in the Workspace behind s, most urgent first and, within a
// severity, the oldest first.
func (m *Module) Items(ctx context.Context, s *ent.Scoped) ([]Item, error) {
	now := m.now()
	readers := []func(context.Context, *ent.Scoped, time.Time) ([]Item, error){
		domainsLost,
		m.integrations,
		heldBroadcasts,
		m.deliverability,
		m.sendCap,
		failedRuns,
		failedBroadcasts,
		disabledWebhooks,
	}
	var items []Item
	for _, read := range readers {
		got, err := read(ctx, s, now)
		if err != nil {
			return nil, err
		}
		items = append(items, got...)
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
			Kind: KindSendingDomainVerificationLost, Severity: SeverityBlocked, Count: 1,
			Subject: &Subject{ID: d.ID, Name: d.Domain}, Since: d.VerificationLostAt,
		})
	}
	return items, nil
}

// integrations reads the standing of every enabled Integration as the provider last reported
// it, and whether it sent its Provider limit over the last 24 hours.
func (m *Module) integrations(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
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
		switch eff.Enforcement {
		case messaging.EnforcementShutdown:
			items = append(items, Item{Kind: KindIntegrationProviderStopped, Severity: SeverityBlocked, Count: 1, Subject: subject})
		case messaging.EnforcementProbation:
			items = append(items, Item{Kind: KindIntegrationProviderProbation, Severity: SeverityDegraded, Count: 1, Subject: subject})
		case messaging.EnforcementNone:
		}
		if eff.Sandbox {
			items = append(items, Item{Kind: KindIntegrationProviderSandbox, Severity: SeverityDegraded, Count: 1, Subject: subject})
		}
		if limit := eff.PerDay.Limit; limit != nil && usage[integ.ID] >= *limit {
			items = append(items, Item{Kind: KindProviderLimitSpent, Severity: SeverityDegraded, Count: 1, Subject: subject})
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
			broadcast.HoldReasonIn(outbound.HoldNoIntegration, outbound.HoldUnverifiedDomain, outbound.HoldProviderStopped),
		).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(held))
	for _, b := range held {
		items = append(items, Item{
			Kind: KindBroadcastHeld, Severity: SeverityBlocked, Count: 1,
			Subject: &Subject{ID: b.ID, Name: b.Name}, HoldReason: b.HoldReason,
		})
	}
	return items, nil
}

func (m *Module) deliverability(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	if m.watch == nil {
		return nil, nil
	}
	w, err := m.watch.Warning(ctx, s, now)
	if err != nil || w == nil {
		return nil, err
	}
	return []Item{{Kind: KindDeliverabilityWarning, Severity: SeverityDegraded, Count: 1, Abuse: w}}, nil
}

func (m *Module) sendCap(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	perDay := m.sendCaps.Of(ws)
	if perDay == nil {
		return nil, nil
	}
	sent, err := providerlimit.SentLast24h(ctx, s, now)
	if err != nil || sent < *perDay {
		return nil, err
	}
	return []Item{{Kind: KindSendCapSpent, Severity: SeverityDegraded, Count: 1}}, nil
}

// failedRuns reads, per Automation, the runs that failed in the last week.
func failedRuns(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	runs, err := s.AutomationRun().Query().
		Where(automationrun.StatusEQ(automationrun.StatusFailed), automationrun.EndedAtGTE(now.Add(-recent))).
		WithAutomation().
		Order(ent.Asc(automationrun.FieldEndedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	var items []Item
	byAutomation := map[int64]int{}
	for _, r := range runs {
		a := r.Edges.Automation
		if a == nil {
			continue
		}
		if i, ok := byAutomation[a.ID]; ok {
			items[i].Count++
			continue
		}
		byAutomation[a.ID] = len(items)
		items = append(items, Item{
			Kind: KindEnrollmentsFailed, Severity: SeverityNotice, Count: 1,
			Subject: &Subject{ID: a.ID, Name: a.Name}, Since: r.EndedAt,
		})
	}
	return items, nil
}

func failedBroadcasts(ctx context.Context, s *ent.Scoped, now time.Time) ([]Item, error) {
	failed, err := s.Broadcast().Query().
		Where(broadcast.StatusEQ(broadcast.StatusFailed), broadcast.UpdatedAtGTE(now.Add(-recent))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(failed))
	for _, b := range failed {
		items = append(items, Item{
			Kind: KindBroadcastFailed, Severity: SeverityNotice, Count: 1,
			Subject: &Subject{ID: b.ID, Name: b.Name}, Since: &b.UpdatedAt,
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
			Kind: KindWebhookDisabled, Severity: SeverityNotice, Count: 1,
			Subject: &Subject{ID: h.ID, Name: h.URL}, Since: h.DisabledAt,
		})
	}
	return items, nil
}
