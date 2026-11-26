// Package frequencycap is the Frequency cap of the sending context (ADR 0137, ADR 0178): the rule
// of a Channel and a country that says which cap holds a Destination, the transaction that takes the
// lock of each Destination a cap holds, and the count of marketing messages per Destination that the
// claim of each writes in it. It owns the count's table; the Workspace's caps are the Workspace's
// settings (internal/domain/identity/workspaces), which it reads.
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

// Claim is a message about to be claimed on Channel: Destinations are the ones it counts at (none
// for a message that does not count).
type Claim struct {
	Channel      channels.Channel
	Destinations []string
}

// Within runs fn in one transaction that first takes the lock of each of in's Destinations a cap
// holds (the rule of in's Channel), and hands it the scope that stands for those locks: the claim and
// its count rows are written there, so two sends to a capped Destination count and claim one after
// the other. A Destination no cap holds takes no lock; with none, fn runs in a transaction that holds
// no lock at all. The locks are held to the commit only: fn calls no provider.
func (m *Module) Within(ctx context.Context, s *ent.Scoped, in Claim, fn func(l *ent.DestinationLocked) error) error {
	keys, err := m.lockKeys(ctx, s, in)
	if err != nil {
		return err
	}
	return m.bus.WithinDestinationLockTx(ctx, s, keys, fn)
}

// lockKeys are the keys of the Destination locks a claim of in takes.
func (m *Module) lockKeys(ctx context.Context, s *ent.Scoped, in Claim) ([]int64, error) {
	if len(in.Destinations) == 0 {
		return nil, nil
	}
	rule, err := ruleOf(ctx, s, in.Channel)
	if err != nil {
		return nil, err
	}
	var keys []int64
	for _, dest := range in.Destinations {
		if rule.locks(dest) {
			keys = append(keys, postgres.DestinationKey(LockKind, s.WorkspaceID(), string(in.Channel), dest))
		}
	}
	return keys, nil
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
	n, err := s.FrequencyCount().Query().Where(
		frequencycount.ChannelEQ(in.Channel),
		frequencycount.Destination(in.Destination),
		frequencycount.CountedAtGT(in.Since),
		frequencycount.HasOutboundMessageWith(outboundmessage.StatusIn(outboundmessage.StatusPending, outboundmessage.StatusSent)),
	).Count(ctx)
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
