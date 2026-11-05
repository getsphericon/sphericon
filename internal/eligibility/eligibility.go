// Package eligibility derives whether a message on a channel from a sending
// source may reach a destination (ADR 0001, 0013). Eligibility is never a stored
// flag on the Contact; it is computed in layers against three destination-keyed
// stores:
//
//  1. (channel, destination) in Suppression            → never (global hard floor)
//  2. (channel, destination) unsubscribed "everything"  → never
//  3. (channel, destination) unsubscribed from source   → never
//  4. no Confirmation, where the Channel asks for one     → never
//  5. otherwise                                         → send
//
// Which Channel asks for a Confirmation (ADR 0077): sms always; email only when the Workspace
// requires confirmed opt-in; a Channel of Bindings never, the Binding being the opt-in.
//
// The layers are defined exactly once (see layers) and consumed in two shapes that
// therefore cannot drift: Check decides one (workspace, destination) pair, and
// Audience narrows a Contact query to the Contacts with an eligible Destination. The rule is
// keyed by destination, never by Contact — a transactional destination may have no Contact.
//
// A source of "" means a transactional send (ADR 0005): it skips layers 2–4 (you
// cannot opt out of, nor must you confirm, your own password reset) but still
// respects Suppression. Whether the workspace requires confirmed opt-in is read
// inside the rule, so no caller can forget the confirmation gate.
//
// Stored destinations and contact.email are both lower-cased on write (contact.email by
// internal/emailcase, held by the contacts_email_lower CHECK), so the Contact-side comparison
// is a plain equality, while Check normalizes its input with suppression.NormalizeDestination.
package eligibility

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	entbinding "github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	entsuppression "github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/suppression"
)

// Reasons a destination is ineligible.
const (
	ReasonSuppressed             = "suppressed"
	ReasonUnsubscribedEverything = "unsubscribed_everything"
	ReasonUnsubscribedSource     = "unsubscribed_source"
	ReasonUnconfirmed            = "unconfirmed"
	// ReasonNoDestination: the Contact has no Destination on the Channel (no email address, no
	// Binding), so there is nothing to send to. The Outbound send records it as a Skipped like an
	// opt-out (ADR 0081).
	ReasonNoDestination = "no_destination"
)

// Decision is the outcome of a single eligibility Check.
type Decision struct {
	Eligible bool
	// Reason is empty when Eligible; otherwise one of the Reason* constants.
	Reason string
}

// Check decides eligibility for one destination in the scoped Workspace. source is
// the Sending source the message goes out under, or "" for a transactional send. An
// empty destination is treated as eligible — the caller decides what a missing
// address means (there is nothing to suppress against).
//
// The rule is one SQL expression (layers) whose Workspace comes from s; the
// check fails closed (an error) for a Workspace that does not exist; the
// confirmation gate reads the workspace flag inside it, so no caller can forget it.
// It is anchored on the Workspace's Suppression query so it runs through the scoped
// client; HAVING TRUE turns the select into a single-group one, so exactly one row
// comes back whether or not the Workspace has any Suppression.
func Check(ctx context.Context, s *ent.Scoped, channel, dest, source string) (Decision, error) {
	d := suppression.NormalizeDestination(dest)
	if d == "" {
		return Decision{Eligible: true}, nil
	}
	r := ref{
		workspace:   func(b *sql.Builder) { b.Arg(s.WorkspaceID()) },
		destination: func(b *sql.Builder) { b.Arg(d) },
	}
	var reason []string
	err := s.Suppression().Query().
		Modify(func(sel *sql.Selector) {
			// raw sql: one CASE over correlated EXISTS subqueries, selected as a column; ent has no
			// conditional expression or subquery select.
			sel.SelectExpr(sql.ExprFunc(func(b *sql.Builder) {
				b.WriteString("CASE WHEN NOT EXISTS (SELECT 1 FROM ").Ident(workspace.Table).
					WriteString(" WHERE ").Ident(workspace.FieldID).WriteString(" = ").Arg(s.WorkspaceID()).
					WriteString(") THEN ").Arg(workspaceMissing)
				for _, l := range layers(r, channel, source) {
					b.WriteString(" WHEN ")
					b.Join(l.blocks)
					b.WriteString(" THEN ").Arg(l.reason)
				}
				b.WriteString(" ELSE '' END")
			}))
			// raw sql: a constant HAVING turns the select into one group; ent has no HAVING TRUE.
			sel.Having(sql.P(func(b *sql.Builder) { b.WriteString("TRUE") }))
		}).
		Scan(ctx, &reason)
	if err != nil {
		return Decision{}, fmt.Errorf("eligibility check: %w", err)
	}
	if len(reason) != 1 {
		return Decision{}, fmt.Errorf("eligibility check: expected one row, got %d", len(reason))
	}
	if reason[0] == workspaceMissing {
		return Decision{}, fmt.Errorf("eligibility check: workspace %d not found", s.WorkspaceID())
	}
	return Decision{Eligible: reason[0] == "", Reason: reason[0]}, nil
}

// workspaceMissing is the sentinel the check expression yields when the scoped
// Workspace does not exist: the check fails closed instead of reporting eligible.
const workspaceMissing = "!workspace"

// A Destination is where a Channel reaches a Contact, as the audience predicate sees it (ADR
// 0075): on email the Contact's own address. The Destination keys the eligibility stores, so the
// predicate correlates on it, never on the Contact's id. A Channel of Bindings reaches a Contact
// at each of its Bindings on the routed Integration, and its Destination adds itself here.
type Destination interface {
	// channel is the Channel whose stores the layers read.
	channel() string
	// reach narrows the outer contacts query to the Contacts that have a Destination for which
	// eligible holds, given the Destination as a ref.
	reach(s *sql.Selector, eligible func(ref) *sql.Predicate)
}

// EmailDestination is the email Destination: the Contact's address, which it must have.
func EmailDestination() Destination { return emailDestination{} }

type emailDestination struct{}

func (emailDestination) channel() string { return ChannelEmail }

func (emailDestination) reach(s *sql.Selector, eligible func(ref) *sql.Predicate) {
	s.Where(sql.And(sql.NotNull(s.C(contact.FieldEmail)), eligible(contactRef(s))))
}

// PhoneDestination is the sms Destination: the Contact's phone, which it must have. It is stored in
// E.164 on every door (ADR 0075), so the stores are matched on it as it is, or on its hash, which
// an erased Contact's opt-outs and Suppressions keep instead (ADR 0021); every layer on sms is keyed
// by the number, never by a sender number.
func PhoneDestination() Destination { return phoneDestination{} }

type phoneDestination struct{}

func (phoneDestination) channel() string { return ChannelSMS }

func (phoneDestination) reach(s *sql.Selector, eligible func(ref) *sql.Predicate) {
	r := ref{
		workspace:   func(b *sql.Builder) { b.WriteString(s.C(contact.FieldWorkspaceID)) },
		destination: func(b *sql.Builder) { b.WriteString(s.C(contact.FieldPhone)) },
	}
	s.Where(sql.And(sql.NotNull(s.C(contact.FieldPhone)), eligible(r)))
}

// BindingDestination is the Destination of a Channel of Bindings (ADR 0075): the Contact is reached
// at each Binding on the Integration, its own and its Visitors', and each one's Destination is
// checked on its own, so a Contact with one eligible Binding is in the audience however many it
// has. An Integration of 0 names none and reaches nobody.
func BindingDestination(channel string, integrationID int64) Destination {
	return bindingDestination{ch: channel, integration: integrationID}
}

type bindingDestination struct {
	ch          string
	integration int64
}

func (d bindingDestination) channel() string { return d.ch }

func (d bindingDestination) reach(s *sql.Selector, eligible func(ref) *sql.Predicate) {
	b := sql.Table(entbinding.Table)
	v := sql.Table(visitor.Table)
	sub := sql.Select(b.C(entbinding.FieldID)).From(b)
	ofVisitors := sql.Select(v.C(visitor.FieldID)).From(v)
	// raw sql: the visitors of the outer Contact, a correlation ent predicates cannot name.
	ofVisitors.Where(sql.P(func(w *sql.Builder) {
		w.WriteString(v.C(visitor.FieldContactID)).WriteString(" = ").WriteString(s.C(contact.FieldID))
	}))
	r := ref{
		workspace:   func(w *sql.Builder) { w.WriteString(b.C(entbinding.FieldWorkspaceID)) },
		destination: func(w *sql.Builder) { w.WriteString(b.C(entbinding.FieldDestination)) },
	}
	sub.Where(sql.And(
		sql.EQ(b.C(entbinding.FieldIntegrationID), d.integration),
		sql.Or(
			// raw sql: the Binding is the outer Contact's own, a correlation ent predicates cannot name.
			sql.P(func(w *sql.Builder) {
				w.WriteString(b.C(entbinding.FieldContactID)).WriteString(" = ").WriteString(s.C(contact.FieldID))
			}),
			sql.In(b.C(entbinding.FieldVisitorID), ofVisitors),
		),
		eligible(r),
	))
	s.Where(sql.Exists(sub))
}

// Audience narrows a Contact query to the Contacts reachable at dest on its Channel from
// source: they have the Destination and it is eligible; "" source means transactional
// (Suppression only). It composes with a Segment predicate (both are predicate.Contact).
func Audience(dest Destination, source string) predicate.Contact {
	return func(s *sql.Selector) {
		dest.reach(s, func(r ref) *sql.Predicate {
			ls := layers(r, dest.channel(), source)
			kept := make([]*sql.Predicate, len(ls))
			for i, l := range ls {
				kept[i] = sql.Not(l.blocks)
			}
			return sql.And(kept...)
		})
	}
}

// GloballyOptedOut matches contacts whose destination is non-mailable on the given
// channel regardless of source: suppressed, or unsubscribed from "everything".
// Used for the "unsubscribed" analytics KPI — the derived, dashboard-level view of
// "fully opted out", since there is no stored contact status (ADR 0001). A
// per-source opt-out is deliberately not counted: that contact may still be
// mailable from other sources.
func GloballyOptedOut(channel string) predicate.Contact {
	return func(s *sql.Selector) {
		r := contactRef(s)
		var preds []*sql.Predicate
		for _, l := range layers(r, channel, SourceBroadcasts) {
			if l.reason == ReasonSuppressed || l.reason == ReasonUnsubscribedEverything {
				preds = append(preds, l.blocks)
			}
		}
		s.Where(sql.Or(preds...))
	}
}

// ref names the (workspace, destination) a layer is evaluated for, as SQL
// expression writers: a correlated Contact column pair for the batch form, bound
// arguments for a single Check.
type ref struct {
	workspace   func(*sql.Builder)
	destination func(*sql.Builder)
}

// contactRef correlates to the outer contacts row and its (lower-cased) email. Writers defer
// column rendering to build time so the outer selector's dialect is applied — eager string
// formatting would emit mis-quoted identifiers.
func contactRef(s *sql.Selector) ref {
	return ref{
		workspace:   func(b *sql.Builder) { b.WriteString(s.C(contact.FieldWorkspaceID)) },
		destination: func(b *sql.Builder) { b.WriteString(s.C(contact.FieldEmail)) },
	}
}

// layer is one rule of the eligibility model: blocks is true when the layer
// forbids the send, reason is what the Decision reports.
type layer struct {
	reason string
	blocks *sql.Predicate
}

// store names the columns of one destination-keyed table a layer reads.
// liveWhenNull lists the columns that are NULL while a row still counts (lifted_at on
// Suppression); a store whose rows never end has none.
type store struct {
	table, id, workspace, destination, channel string
	liveWhenNull                               []string
}

var (
	suppressions  = store{entsuppression.Table, entsuppression.FieldID, entsuppression.FieldWorkspaceID, entsuppression.FieldDestination, entsuppression.FieldChannel, []string{entsuppression.FieldLiftedAt}}
	unsubscribes  = store{unsubscribe.Table, unsubscribe.FieldID, unsubscribe.FieldWorkspaceID, unsubscribe.FieldDestination, unsubscribe.FieldChannel, nil}
	confirmations = store{confirmation.Table, confirmation.FieldID, confirmation.FieldWorkspaceID, confirmation.FieldDestination, confirmation.FieldChannel, nil}
)

// layers is the single definition of the eligibility rule, in precedence order
// (negatives first, the positive confirmation gate last so the negatives always
// dominate). source "" is a transactional send.
func layers(r ref, channel, source string) []layer {
	suppressed := layer{
		reason: ReasonSuppressed,
		blocks: sql.Exists(match(r, suppressions, channel)),
	}
	if source == "" {
		return []layer{suppressed}
	}
	out := make([]layer, 0, 4)
	out = append(out, suppressed,
		layer{
			reason: ReasonUnsubscribedEverything,
			blocks: sql.Exists(match(r, unsubscribes, channel, unsubscribe.FieldSendingSource, SourceEverything)),
		},
		layer{
			reason: ReasonUnsubscribedSource,
			blocks: sql.Exists(match(r, unsubscribes, channel, unsubscribe.FieldSendingSource, source)),
		})
	unconfirmed := sql.NotExists(match(r, confirmations, channel))
	switch channel {
	case ChannelTelegram:
		// A Binding is the opt-in of a Channel of Bindings: there is no Confirmation to ask for.
		return out
	case ChannelSMS:
		// A text asks for a Confirmation always (ADR 0077): the Workspace's confirmed-opt-in
		// setting is about email only, and turning it off never relaxes sms.
		return append(out, layer{reason: ReasonUnconfirmed, blocks: unconfirmed})
	}
	return append(out, layer{
		reason: ReasonUnconfirmed,
		// Only when the workspace has confirmed opt-in on (ADR 0013): the flag is
		// read in SQL so the default single-opt-in path never touches the
		// confirmation store and no caller can forget to pass it.
		blocks: sql.And(requireConfirmed(r), unconfirmed),
	})
}

// match builds SELECT id FROM store WHERE workspace/destination/channel equal the
// ref [AND extraCol = extraVal]. Columns of the store's own table are qualified
// eagerly; the ref (outer) side is rendered lazily so the dialect is applied.
func match(r ref, st store, channel string, extra ...string) *sql.Selector {
	sub := sql.Select(st.id).From(sql.Table(st.table))
	conds := []*sql.Predicate{
		// raw sql: the right side is a column of the outer query (or a bound value) rendered at build
		// time, a correlation ent predicates cannot name.
		sql.P(func(b *sql.Builder) { b.WriteString(sub.C(st.workspace)).WriteString(" = "); r.workspace(b) }),
		// raw sql: the same correlation, on the destination.
		sql.P(func(b *sql.Builder) { b.WriteString(sub.C(st.destination)).WriteString(" = "); r.destination(b) }),
		sql.EQ(sub.C(st.channel), channel),
	}
	if channel == ChannelSMS {
		// On sms a row is keyed by the number or, once its Contact was erased, by the number's hash
		// (phone.Hash, here in SQL), so an erased person's refusal holds when the number comes back.
		// raw sql: the destination correlation, matched against the number or its SHA-256 hex.
		conds[1] = sql.P(func(b *sql.Builder) {
			b.WriteString(sub.C(st.destination)).WriteString(" IN (")
			r.destination(b)
			b.WriteString(", encode(sha256(convert_to(")
			r.destination(b)
			b.WriteString(", 'UTF8')), 'hex'))")
		})
	}
	for _, col := range st.liveWhenNull {
		conds = append(conds, sql.IsNull(sub.C(col)))
	}
	if len(extra) == 2 {
		conds = append(conds, sql.EQ(sub.C(extra[0]), extra[1]))
	}
	return sub.Where(sql.And(conds...))
}

// requireConfirmed is true when the referenced workspace has confirmed opt-in on.
func requireConfirmed(r ref) *sql.Predicate {
	// raw sql: a scalar subquery inside COALESCE used as a predicate; ent has no subquery predicate
	// over another table.
	return sql.P(func(b *sql.Builder) {
		sub := sql.Select(workspace.FieldRequireConfirmedOptIn).From(sql.Table(workspace.Table))
		// raw sql: correlates the subquery with the outer query's workspace column.
		sub.Where(sql.P(func(pb *sql.Builder) {
			pb.WriteString(sub.C(workspace.FieldID)).WriteString(" = ")
			r.workspace(pb)
		}))
		b.WriteString("COALESCE((").Join(sub).WriteString("), FALSE)")
	})
}
