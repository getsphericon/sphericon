// Package frequencycap is the Frequency cap of the sending context (ADR 0137, ADR 0178): the rule
// of a Channel and a country that says which cap holds a Destination, the transaction that takes the
// lock of each Destination a cap holds and reads under it where each stands against its cap, the
// count of marketing messages per Destination that the claim of each writes in it, and a read-only
// probe of the same standing at any moment, for a preview and the reports. It owns the count's
// table; the Workspace's caps are the Workspace's settings (internal/domain/identity/workspaces),
// which it reads.
package frequencycap

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/getsphericon/sphericon/ent"
	entcap "github.com/getsphericon/sphericon/ent/frequencycap"
	"github.com/getsphericon/sphericon/ent/frequencycount"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/drain"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// LockKind is the kind of the Destination lock the count and the claim are written under
// (postgres.DestinationKey): a race test queues a send behind the key of this kind.
const LockKind = "frequency-cap"

// Retention is how long a count row is kept: the longest period a cap may have, past which no cap
// counts it.
const Retention = bounds.FrequencyCapPeriodMinutesMax * time.Minute

// Module is the Frequency cap module.
type Module struct {
	bus *events.Bus
	// now is the Instance clock: the moment a message is counted at, and the age of the rows pruned.
	now func() time.Time
}

// New builds the module over the bus its lock transactions run on and the Instance clock.
func New(bus *events.Bus, now func() time.Time) *Module {
	return &Module{bus: bus, now: now}
}

// Cap is a Frequency cap: at most Max marketing messages to one Destination per rolling Period.
type Cap struct {
	Max    int
	Period time.Duration
}

// Rule is the Frequency cap rule of a Workspace's Channel (ADR 0137, ADR 0178): which of its caps
// holds a Destination. A country's row outranks the Channel's for a number of that country, and a
// row with no maximum removes the cap there.
type Rule struct {
	channel channels.Channel
	// channelRow is the Channel's own row, nil without one; countryRows the rows of countries. A
	// row's Cap is nil when the row removes the cap.
	channelRow  *row
	countryRows map[string]row
}

// row is one of the Workspace's cap rows: its cap, nil for a row that removes the cap.
type row struct{ cap *Cap }

// Cap is the cap that holds dest on the rule's Channel, and whether one does: the row of the
// Destination's country when the Channel takes one (a number's, workspaces.TakesCountry) and the
// Workspace has a row for it, else the Channel's own.
func (r Rule) Cap(dest string) (Cap, bool) {
	if workspaces.TakesCountry(r.channel) {
		if country := phone.Country(dest); country != "" {
			if c, ok := r.countryRows[country]; ok {
				return capOf(&c)
			}
		}
	}
	return capOf(r.channelRow)
}

// capOf is the cap of the row, none when there is no row or it removes the cap.
func capOf(r *row) (Cap, bool) {
	if r == nil || r.cap == nil {
		return Cap{}, false
	}
	return *r.cap, true
}

// locks reports whether a claim for dest takes the Destination's lock: when a cap holds it, so the
// count against it and the claim are exact; elsewhere the count needs no lock.
func (r Rule) locks(dest string) bool {
	_, capped := r.Cap(dest)
	return capped
}

// Rule reads the Frequency cap rule of the scoped Workspace's Channel ch: which of its caps holds
// each Destination.
func (m *Module) Rule(ctx context.Context, s *ent.Scoped, ch channels.Channel) (Rule, error) {
	return ruleOf(ctx, s, ch)
}

// ruleOf reads the rule of the scoped Workspace's Channel ch from its cap rows.
func ruleOf(ctx context.Context, s *ent.Scoped, ch channels.Channel) (Rule, error) {
	rows, err := s.FrequencyCap().Query().Where(entcap.ChannelEQ(ch)).All(ctx)
	if err != nil {
		return Rule{}, fmt.Errorf("frequencycap: read the caps: %w", err)
	}
	rule := Rule{channel: ch, countryRows: map[string]row{}}
	for _, c := range rows {
		r := row{}
		if c.MaxMessages != nil && c.PeriodMinutes != nil {
			r.cap = &Cap{Max: *c.MaxMessages, Period: time.Duration(*c.PeriodMinutes) * time.Minute}
		}
		if c.Country == nil {
			rule.channelRow = &r
			continue
		}
		rule.countryRows[*c.Country] = r
	}
	return rule, nil
}

// Standing is where a Destination stands against its Frequency cap at a moment.
type Standing struct {
	// Cap is the cap that holds the Destination, and Capped whether one does; without one the
	// Destination is never over and the rest is zero.
	Cap    Cap
	Capped bool
	// Count is how many places the Destination's count holds in the cap's period up to the moment:
	// the rows of its messages still pending or sent.
	Count int
	// Release is, for a Destination over its cap, when enough of its counted places leave the period
	// for one to free: the counted moment of the (Count - Max + 1)-th oldest of them plus the period.
	// It is zero while a place is free.
	Release time.Time
}

// Over reports whether the Destination has no place left under its cap: a message to it now would
// go beyond the cap.
func (st Standing) Over() bool { return st.Capped && st.Count >= st.Cap.Max }

// Standings are the Standing of each of a set of Destinations, by Destination.
type Standings map[string]Standing

// Over reports whether dest has no place left under its cap; a Destination not in the set has one.
func (sts Standings) Over(dest string) bool { return sts[dest].Over() }

// standingsAt reads where each of dests on ch stands at the moment at under rule, through s: in the
// transaction that holds their locks for a claim, or outside any for a probe. A probe reads only the
// rows counted up to at (upTo), so a moment past reads as it stood; a claim counts every place in the
// period, so a row another worker wrote by a clock a little ahead of this one's still holds.
func standingsAt(ctx context.Context, s *ent.Scoped, rule Rule, ch channels.Channel, dests []string, at time.Time, upTo bool) (Standings, error) {
	sts := Standings{}
	var capped []string
	var longest time.Duration
	for _, dest := range dests {
		c, ok := rule.Cap(dest)
		sts[dest] = Standing{Cap: c, Capped: ok}
		if ok {
			capped = append(capped, dest)
			longest = max(longest, c.Period)
		}
	}
	if len(capped) == 0 {
		return sts, nil
	}
	where := append(holdingPlaces(ch, at.Add(-longest)), frequencycount.DestinationIn(capped...))
	if upTo {
		where = append(where, frequencycount.CountedAtLTE(at))
	}
	rows, err := s.FrequencyCount().Query().Where(where...).Order(frequencycount.ByCountedAt()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("frequencycap: read the counts: %w", err)
	}
	for _, dest := range capped {
		sts[dest] = standingOf(sts[dest], rows, dest, at)
	}
	return sts, nil
}

// standingOf is st with the count of dest's rows (oldest first) inside its cap's period up to at,
// and its release moment when the count is at or over the cap.
func standingOf(st Standing, rows []*ent.FrequencyCount, dest string, at time.Time) Standing {
	since := at.Add(-st.Cap.Period)
	var counted []time.Time
	for _, r := range rows {
		if r.Destination == dest && r.CountedAt.After(since) {
			counted = append(counted, r.CountedAt)
		}
	}
	st.Count = len(counted)
	if st.Over() && st.Cap.Max > 0 {
		st.Release = counted[st.Count-st.Cap.Max].Add(st.Cap.Period)
	}
	return st
}

// holdingPlaces is the condition of the count rows of ch that hold a place after since: counted
// after it, of a message still pending or sent. A message that failed or was skipped frees its
// places.
func holdingPlaces(ch channels.Channel, since time.Time) []predicate.FrequencyCount {
	return []predicate.FrequencyCount{
		frequencycount.ChannelEQ(ch),
		frequencycount.CountedAtGT(since),
		frequencycount.HasOutboundMessageWith(outboundmessage.StatusIn(outboundmessage.StatusPending, outboundmessage.StatusSent)),
	}
}

// ProbeInput is a set of Destinations on Channel, and the moment At they are read at.
type ProbeInput struct {
	Channel      channels.Channel
	Destinations []string
	At           time.Time
}

// Probe reads where each of in's Destinations stands against its cap at in.At: its count and, over
// the cap, when a place frees. It takes no lock and writes nothing, so its answer may be stale by the
// time a send claims: for a preview or a report, never for the claim (Within).
func (m *Module) Probe(ctx context.Context, s *ent.Scoped, in ProbeInput) (Standings, error) {
	rule, err := ruleOf(ctx, s, in.Channel)
	if err != nil {
		return nil, err
	}
	return standingsAt(ctx, s, rule, in.Channel, in.Destinations, in.At, true)
}

// Claim is a message about to be claimed on Channel: Destinations are the ones it counts at (none
// for a message that does not count).
type Claim struct {
	Channel      channels.Channel
	Destinations []string
}

// Within runs fn in one transaction that first takes the lock of each of in's Destinations a cap
// holds (the rule of in's Channel), and hands it the scope that stands for those locks and the
// Standing of each Destination read under them, at the Instance clock's now: the claim and its count
// rows are written there, only at the Destinations with a place, so two sends to a capped
// Destination count and claim one after the other and cannot both take its last place. A
// Destination no cap holds takes no lock; with none, fn runs in a transaction that holds no lock at
// all. The locks are held to the commit only: fn calls no provider.
func (m *Module) Within(ctx context.Context, s *ent.Scoped, in Claim, fn func(l *ent.DestinationLocked, sts Standings) error) error {
	if len(in.Destinations) == 0 {
		return m.bus.WithinDestinationLockTx(ctx, s, nil, func(l *ent.DestinationLocked) error { return fn(l, Standings{}) })
	}
	rule, err := ruleOf(ctx, s, in.Channel)
	if err != nil {
		return err
	}
	var keys []int64
	for _, dest := range in.Destinations {
		if rule.locks(dest) {
			keys = append(keys, postgres.DestinationKey(LockKind, s.WorkspaceID(), string(in.Channel), dest))
		}
	}
	return m.bus.WithinDestinationLockTx(ctx, s, keys, func(l *ent.DestinationLocked) error {
		sts, err := standingsAt(ctx, l.Scope(), rule, in.Channel, in.Destinations, m.now(), false)
		if err != nil {
			return err
		}
		return fn(l, sts)
	})
}

// CountInput is a claimed message to count: one place on Channel at each of its Destinations.
type CountInput struct {
	MessageID    int64
	Channel      channels.Channel
	Destinations []string
}

// Count writes the count rows of a new claim, in the transaction that writes it under the
// Destinations' locks (Within): one per Destination, counted now, whether or not a cap holds it. A
// Destination listed twice counts once.
func (m *Module) Count(ctx context.Context, l *ent.DestinationLocked, in CountInput) error {
	at := m.now().Truncate(time.Microsecond)
	dests := slices.Compact(slices.Sorted(slices.Values(in.Destinations)))
	creates := make([]*ent.FrequencyCountScopedCreate, 0, len(dests))
	for _, dest := range dests {
		creates = append(creates, l.FrequencyCount().Create().
			SetOutboundMessageID(in.MessageID).SetChannel(in.Channel).SetDestination(dest).SetCountedAt(at))
	}
	if err := l.FrequencyCount().CreateBulk(creates...).Exec(ctx); err != nil {
		return fmt.Errorf("frequencycap: count message %d: %w", in.MessageID, err)
	}
	return nil
}

// PlacesOf is the Destinations the message holds a place at: those its claim was counted at, less
// any the provider dropped since. A claim taken over goes to these alone, the ones its places were
// taken for under the cap.
func (m *Module) PlacesOf(ctx context.Context, s *ent.Scoped, messageID int64) ([]string, error) {
	dests, err := s.FrequencyCount().Query().
		Where(frequencycount.OutboundMessageID(messageID)).
		Select(frequencycount.FieldDestination).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("frequencycap: read the places of message %d: %w", messageID, err)
	}
	return dests, nil
}

// UncountInput is a target the provider dropped from a message: the Destination it no longer
// reaches.
type UncountInput struct {
	MessageID   int64
	Destination string
}

// Uncount frees the place a message took at a Destination the provider dropped (a Binding gone, an
// opt-out at the provider): the message never reaches it.
func (m *Module) Uncount(ctx context.Context, s *ent.Scoped, in UncountInput) error {
	_, err := s.FrequencyCount().Delete().Where(
		frequencycount.OutboundMessageID(in.MessageID), frequencycount.Destination(in.Destination)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("frequencycap: uncount message %d: %w", in.MessageID, err)
	}
	return nil
}

// CountedInput is a Destination on a Channel, and the moment from which its count is read.
type CountedInput struct {
	Channel     channels.Channel
	Destination string
	Since       time.Time
}

// Counted is how many places the Destination's count holds after in.Since: the rows of its messages
// still pending or sent. A message that failed or was skipped frees its places.
func (m *Module) Counted(ctx context.Context, s *ent.Scoped, in CountedInput) (int, error) {
	n, err := s.FrequencyCount().Query().
		Where(append(holdingPlaces(in.Channel, in.Since), frequencycount.Destination(in.Destination))...).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("frequencycap: read the count: %w", err)
	}
	return n, nil
}

// Prune deletes the count rows of every Workspace counted more than Retention before the Instance
// clock's now, in batches of batchSize, and returns how many it removed: no cap counts them any
// more. It spans every Workspace, so it takes the raw client.
func (m *Module) Prune(ctx context.Context, client *ent.Client, batchSize int) (int64, error) {
	cutoff := m.now().Add(-Retention)
	total, err := drain.Batches(batchSize, func() (int, error) {
		ids, err := client.FrequencyCount.Query().
			Where(frequencycount.CountedAtLT(cutoff)).
			Limit(batchSize).
			IDs(ctx)
		if err != nil {
			return 0, fmt.Errorf("frequencycap: select expired count rows: %w", err)
		}
		if len(ids) == 0 {
			return 0, nil
		}
		n, err := client.FrequencyCount.Delete().Where(frequencycount.IDIn(ids...)).Exec(ctx)
		if err != nil {
			return 0, fmt.Errorf("frequencycap: delete expired count rows: %w", err)
		}
		return n, nil
	})
	return int64(total), err
}
