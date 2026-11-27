package eligibility

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	entbinding "github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// State is a Destination's standing on its Channel, as Send eligibility decides a Broadcast to it
// (ADR 0138, 0141): the first layer that blocks it, or that it may receive.
type State string

const (
	// StateNone is a Contact with no Destination on the Channel.
	StateNone State = ""
	// StateMayReceive is a Destination a Broadcast reaches.
	StateMayReceive State = "may_receive"
	// StateNeedsConfirmation is a Destination the Channel asks a Confirmation of, and that has none.
	StateNeedsConfirmation State = "needs_confirmation"
	// StateOptedOut is a Destination opted out of everything or of Broadcasts.
	StateOptedOut State = "opted_out"
	// StateSuppressed is a Destination with a Suppression standing.
	StateSuppressed State = "suppressed"
)

// OptOut is one scope a Destination opted out of (a Sending source, or everything) and when the
// person stopped (ADR 0154).
type OptOut struct {
	Scope string
	At    time.Time
}

// SuppressionFact is the Suppression standing on a Destination: its reason and when it was made.
type SuppressionFact struct {
	Reason entsuppression.Reason
	At     time.Time
}

// ConfirmationFact is the Confirmation a Destination holds: how it was obtained and when.
type ConfirmationFact struct {
	Provenance confirmation.Provenance
	At         time.Time
}

// ChannelState is a Contact's standing on one Channel: its Destination there ("" for none, with
// StateNone), its State, and what stands behind it, each read whatever the State is: a suppressed
// Destination still lists the scopes it opted out of.
type ChannelState struct {
	Destination string
	State       State
	// OptOuts are the scopes the Destination opted out of, ordered by scope.
	OptOuts      []OptOut
	Suppression  *SuppressionFact
	Confirmation *ConfirmationFact
}

// ContactStates is a Contact's standing on each Channel of consent: email at its address, sms at
// its phone, Telegram at its Bindings.
type ContactStates struct {
	Email    ChannelState
	SMS      ChannelState
	Telegram ChannelState
}

// broadcastMode is the mode a Channel state is decided in: a Broadcast's.
var broadcastMode = Marketing(SourceBroadcasts)

// ChannelStates reads the standing of each of contacts on email, sms and Telegram, keyed by
// Contact id: one entry per Contact given. The State is Send eligibility's for a Broadcast,
// selected from the same layers a Check and an Audience read, so the three never disagree; the
// opt-outs, Suppression and Confirmation beside it are the rows those layers match (on sms by the
// number or its hash). The read is set-based: its queries are as many for a page of thousands as
// for one Contact. On Telegram a Contact is reached at each of its Bindings, and its state is
// that of the Binding that fares best (the first that may receive, in the order they were made),
// as an Audience keeps a Contact with one eligible Binding however many it has.
func (m *Module) ChannelStates(ctx context.Context, s *ent.Scoped, contacts []*ent.Contact) (map[int64]ContactStates, error) {
	out := make(map[int64]ContactStates, len(contacts))
	if len(contacts) == 0 {
		return out, nil
	}
	ids := lo.Map(contacts, func(c *ent.Contact, _ int) int64 { return c.ID })
	email, err := contactStates(ctx, s, ids, contact.FieldEmail, channels.Email)
	if err != nil {
		return nil, err
	}
	sms, err := contactStates(ctx, s, ids, contact.FieldPhone, channels.SMS)
	if err != nil {
		return nil, err
	}
	telegram, err := bindingStates(ctx, s, ids)
	if err != nil {
		return nil, err
	}
	dests := map[channels.Channel][]string{
		channels.Email:    lo.Map(email, func(r stateRow, _ int) string { return r.Destination }),
		channels.SMS:      lo.FlatMap(sms, func(r stateRow, _ int) []string { return []string{r.Destination, phone.Hash(r.Destination)} }),
		channels.Telegram: lo.Map(telegram, func(r stateRow, _ int) string { return r.Destination }),
	}
	f, err := readFacts(ctx, s, dests)
	if err != nil {
		return nil, err
	}
	for _, c := range contacts {
		out[c.ID] = ContactStates{}
	}
	for _, r := range email {
		cs := out[r.ContactID]
		cs.Email = f.state(channels.Email, r)
		out[r.ContactID] = cs
	}
	for _, r := range sms {
		cs := out[r.ContactID]
		cs.SMS = f.state(channels.SMS, r)
		out[r.ContactID] = cs
	}
	for _, r := range telegram {
		cs := out[r.ContactID]
		cs.Telegram = better(cs.Telegram, f.state(channels.Telegram, r))
		out[r.ContactID] = cs
	}
	return out, nil
}

// stateRow is one Destination of a Contact with the State the layers select for it.
type stateRow struct {
	ContactID   int64  `sql:"contact_id"`
	Destination string `sql:"destination"`
	State       string `sql:"state"`
}

// stateExpr selects the State the layers give the Destination r names, on channel, for a Broadcast.
func stateExpr(r ref, channel channels.Channel) sql.Querier {
	// raw sql: one CASE over the layers' correlated EXISTS subqueries, selected as a column; ent has
	// no conditional expression.
	return sql.ExprFunc(func(b *sql.Builder) {
		firstBlocking(b, layers(r, channel, broadcastMode), func(l layer) string { return string(l.state) }, string(StateMayReceive))
	})
}

// contactStates reads the State of the Contacts' own Destination in column (email or phone), for
// those that have one.
func contactStates(ctx context.Context, s *ent.Scoped, ids []int64, column string, channel channels.Channel) ([]stateRow, error) {
	var rows []stateRow
	err := s.Contact().Query().
		Where(contact.IDIn(ids...), func(sel *sql.Selector) { sel.Where(sql.NotNull(sel.C(column))) }).
		Modify(func(sel *sql.Selector) {
			r := ref{
				workspace:   func(b *sql.Builder) { b.WriteString(sel.C(contact.FieldWorkspaceID)) },
				destination: func(b *sql.Builder) { b.WriteString(sel.C(column)) },
			}
			sel.Select(sql.As(sel.C(contact.FieldID), "contact_id"), sql.As(sel.C(column), "destination"))
			sel.AppendSelectExprAs(stateExpr(r, channel), "state")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("channel states on %s: %w", channel, err)
	}
	return rows, nil
}

// bindingStates reads the State of each Telegram Binding of the Contacts, in the order they were
// made.
func bindingStates(ctx context.Context, s *ent.Scoped, ids []int64) ([]stateRow, error) {
	var rows []stateRow
	err := s.Binding().Query().
		Where(entbinding.ContactIDIn(ids...), entbinding.ChannelEQ(entbinding.ChannelTelegram)).
		Order(ent.Asc(entbinding.FieldID)).
		Modify(func(sel *sql.Selector) {
			r := ref{
				workspace:   func(b *sql.Builder) { b.WriteString(sel.C(entbinding.FieldWorkspaceID)) },
				destination: func(b *sql.Builder) { b.WriteString(sel.C(entbinding.FieldDestination)) },
			}
			sel.Select(sel.C(entbinding.FieldContactID), sel.C(entbinding.FieldDestination))
			sel.AppendSelectExprAs(stateExpr(r, channels.Telegram), "state")
		}).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("channel states on telegram: %w", err)
	}
	return rows, nil
}

// rank orders States from the best for a Contact to the worst: the Binding a Contact's Telegram
// state reads is the first of the best rank.
func rank(st State) int {
	switch st {
	case StateMayReceive:
		return 0
	case StateNeedsConfirmation:
		return 1
	case StateOptedOut:
		return 2
	case StateSuppressed:
		return 3
	case StateNone:
	}
	return 4
}

// better is the state of the two that fares better, the first of an equal rank: the one of the
// Binding made earlier.
func better(first, next ChannelState) ChannelState {
	if rank(next.State) < rank(first.State) {
		return next
	}
	return first
}

// destKey names a Destination on a Channel.
type destKey struct {
	channel     channels.Channel
	destination string
}

// facts are the rows behind the Destinations' states, by the Destination each is keyed by.
type facts struct {
	optOuts       map[destKey][]*ent.Unsubscribe
	suppressions  map[destKey][]*ent.Suppression
	confirmations map[destKey][]*ent.Confirmation
	// stamps are the opt-outs' consent stamps, by their stored key and Sending source.
	stamps map[destKey]map[string]*ent.ConsentStamp
}

// readFacts reads, in one query a store, the opt-outs, the standing Suppressions, the
// Confirmations and the consent stamps of the Destinations, in the order they were made.
func readFacts(ctx context.Context, s *ent.Scoped, dests map[channels.Channel][]string) (facts, error) {
	// of matches the rows of a consent table keyed by one of the Destinations on their Channel: each
	// table names its columns as the Unsubscribe's do.
	of := func(sel *sql.Selector) {
		ps := make([]*sql.Predicate, 0, len(dests))
		for _, ch := range []channels.Channel{channels.Email, channels.SMS, channels.Telegram} {
			ps = append(ps, sql.And(
				sql.EQ(sel.C(unsubscribe.FieldChannel), string(ch)),
				sql.In(sel.C(unsubscribe.FieldDestination), lo.ToAnySlice(dests[ch])...),
			))
		}
		sel.Where(sql.Or(ps...))
	}
	f := facts{stamps: map[destKey]map[string]*ent.ConsentStamp{}}
	optOuts, err := s.Unsubscribe().Query().Where(of).Order(ent.Asc(unsubscribe.FieldID)).All(ctx)
	if err != nil {
		return facts{}, fmt.Errorf("channel states' opt-outs: %w", err)
	}
	f.optOuts = lo.GroupBy(optOuts, func(u *ent.Unsubscribe) destKey { return destKey{u.Channel, u.Destination} })
	suppressions, err := s.Suppression().Query().
		Where(of, entsuppression.LiftedAtIsNil()).Order(ent.Asc(entsuppression.FieldID)).All(ctx)
	if err != nil {
		return facts{}, fmt.Errorf("channel states' suppressions: %w", err)
	}
	f.suppressions = lo.GroupBy(suppressions, func(u *ent.Suppression) destKey { return destKey{u.Channel, u.Destination} })
	confirmations, err := s.Confirmation().Query().Where(of).Order(ent.Asc(confirmation.FieldID)).All(ctx)
	if err != nil {
		return facts{}, fmt.Errorf("channel states' confirmations: %w", err)
	}
	f.confirmations = lo.GroupBy(confirmations, func(u *ent.Confirmation) destKey { return destKey{u.Channel, u.Destination} })
	stamped, err := s.ConsentStamp().Query().Where(of).All(ctx)
	if err != nil {
		return facts{}, fmt.Errorf("channel states' consent stamps: %w", err)
	}
	for _, st := range stamped {
		k := destKey{st.Channel, st.Destination}
		if f.stamps[k] == nil {
			f.stamps[k] = map[string]*ent.ConsentStamp{}
		}
		f.stamps[k][st.SendingSource] = st
	}
	return f, nil
}

// keys are the keys a Destination's rows may be kept by, as the layers match them: on sms the
// number or its hash.
func keys(channel channels.Channel, destination string) []destKey {
	k := destKey{channel, destination}
	if channel == channels.SMS {
		return []destKey{k, {channel, phone.Hash(destination)}}
	}
	return []destKey{k}
}

// state is the ChannelState of the Destination r, with its rows.
func (f facts) state(channel channels.Channel, r stateRow) ChannelState {
	out := ChannelState{Destination: r.Destination, State: State(r.State)}
	ks := keys(channel, r.Destination)
	// A stamp is kept by the number's hash on sms (consentstamps.Stored), whichever key its
	// opt-out is kept by.
	stamps := f.stamps[ks[len(ks)-1]]
	seen := map[string]bool{}
	for _, k := range ks {
		for _, u := range f.optOuts[k] {
			if seen[u.SendingSource] {
				continue
			}
			seen[u.SendingSource] = true
			at := u.CreatedAt
			if st := stamps[u.SendingSource]; st != nil && st.OptOutOccurredAt != nil {
				at = *st.OptOutOccurredAt
			}
			out.OptOuts = append(out.OptOuts, OptOut{Scope: u.SendingSource, At: at.UTC()})
		}
		if sup := lo.FirstOr(f.suppressions[k], nil); sup != nil && out.Suppression == nil {
			out.Suppression = &SuppressionFact{Reason: sup.Reason, At: sup.CreatedAt.UTC()}
		}
		if c := lo.FirstOr(f.confirmations[k], nil); c != nil && out.Confirmation == nil {
			out.Confirmation = &ConfirmationFact{Provenance: c.Provenance, At: c.CreatedAt.UTC()}
		}
	}
	slices.SortFunc(out.OptOuts, func(a, b OptOut) int { return cmp.Compare(a.Scope, b.Scope) })
	return out
}
