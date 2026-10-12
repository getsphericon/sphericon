// Package eligibility derives whether a message on a channel from a sending
// source may reach a destination (ADR 0001, 0013). Eligibility is never a stored
// flag on the Contact; it is computed in layers against three destination-keyed
// stores:
//
//  1. (channel, destination) in Suppression            → never (global hard floor)
//  2. (channel, destination) unsubscribed "everything"  → never
//  3. (channel, destination) unsubscribed from source   → never
//  4. workspace requires confirmed opt-in and no Confirmation → never
//  5. otherwise                                         → send
//
// The layers are defined exactly once (see layers) and consumed in two shapes that
// therefore cannot drift: Check decides one (workspace, destination) pair, and
// Predicate narrows a Contact query to the eligible audience. The rule is keyed by
// destination, never by Contact — a transactional destination may have no Contact.
//
// A source of "" means a transactional send (ADR 0005): it skips layers 2–4 (you
// cannot opt out of, nor must you confirm, your own password reset) but still
// respects Suppression. Whether the workspace requires confirmed opt-in is read
// inside the rule, so no caller can forget the confirmation gate.
//
// Stored destinations are normalized (lower-cased + trimmed) on write, but
// contact.email is not, so the Contact-side comparison folds case in SQL while
// Check normalizes its input with NormalizeDestination.
package eligibility

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/ent/workspace"
)

// Reasons a destination is ineligible.
const (
	ReasonSuppressed             = "suppressed"
	ReasonUnsubscribedEverything = "unsubscribed_everything"
	ReasonUnsubscribedSource     = "unsubscribed_source"
	ReasonUnconfirmed            = "unconfirmed"
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
	d := NormalizeDestination(dest)
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

// Predicate narrows a Contact query to contacts whose email destination is
// eligible for (channel, source); "" source means transactional (Suppression
// only). It composes with a segment predicate (both are predicate.Contact). The
// correlation joins on lower(contacts.email) — never on contact_id — because the
// stores are destination-keyed.
func Predicate(channel, source string) predicate.Contact {
	return func(s *sql.Selector) {
		r := contactRef(s)
		for _, l := range layers(r, channel, source) {
			s.Where(sql.Not(l.blocks))
		}
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

// contactRef correlates to the outer contacts row, folding case on the
// (un-normalized) contact email. Writers defer column rendering to build time so
// the outer selector's dialect is applied — eager string formatting would emit
// mis-quoted identifiers.
func contactRef(s *sql.Selector) ref {
	return ref{
		workspace: func(b *sql.Builder) { b.WriteString(s.C(contact.FieldWorkspaceID)) },
		destination: func(b *sql.Builder) {
			b.WriteString("lower(").WriteString(s.C(contact.FieldEmail)).WriteString(")")
		},
	}
}

// layer is one rule of the eligibility model: blocks is true when the layer
// forbids the send, reason is what the Decision reports.
type layer struct {
	reason string
	blocks *sql.Predicate
}

// store names the columns of one destination-keyed table a layer reads.
type store struct{ table, id, workspace, destination, channel string }

var (
	suppressions  = store{suppression.Table, suppression.FieldID, suppression.FieldWorkspaceID, suppression.FieldDestination, suppression.FieldChannel}
	unsubscribes  = store{unsubscribe.Table, unsubscribe.FieldID, unsubscribe.FieldWorkspaceID, unsubscribe.FieldDestination, unsubscribe.FieldChannel}
	confirmations = store{confirmation.Table, confirmation.FieldID, confirmation.FieldWorkspaceID, confirmation.FieldDestination, confirmation.FieldChannel}
)

// layers is the single definition of the eligibility rule, in precedence order
// (negatives first, the positive confirmation gate last so the negatives always
// dominate). source "" is a transactional send.
func layers(r ref, channel, source string) []layer {
	out := []layer{{
		reason: ReasonSuppressed,
		blocks: sql.Exists(match(r, suppressions, channel)),
	}}
	if source == "" {
		return out
	}
	return append(out,
		layer{
			reason: ReasonUnsubscribedEverything,
			blocks: sql.Exists(match(r, unsubscribes, channel, unsubscribe.FieldSendingSource, SourceEverything)),
		},
		layer{
			reason: ReasonUnsubscribedSource,
			blocks: sql.Exists(match(r, unsubscribes, channel, unsubscribe.FieldSendingSource, source)),
		},
		layer{
			reason: ReasonUnconfirmed,
			// Only when the workspace has confirmed opt-in on (ADR 0013): the flag is
			// read in SQL so the default single-opt-in path never touches the
			// confirmation store and no caller can forget to pass it.
			blocks: sql.And(requireConfirmed(r), sql.NotExists(match(r, confirmations, channel))),
		},
	)
}

// match builds SELECT id FROM store WHERE workspace/destination/channel equal the
// ref [AND extraCol = extraVal]. Columns of the store's own table are qualified
// eagerly; the ref (outer) side is rendered lazily so the dialect is applied.
func match(r ref, st store, channel string, extra ...string) *sql.Selector {
	sub := sql.Select(st.id).From(sql.Table(st.table))
	conds := []*sql.Predicate{
		sql.P(func(b *sql.Builder) { b.WriteString(sub.C(st.workspace)).WriteString(" = "); r.workspace(b) }),
		sql.P(func(b *sql.Builder) { b.WriteString(sub.C(st.destination)).WriteString(" = "); r.destination(b) }),
		sql.EQ(sub.C(st.channel), channel),
	}
	if len(extra) == 2 {
		conds = append(conds, sql.EQ(sub.C(extra[0]), extra[1]))
	}
	return sub.Where(sql.And(conds...))
}

// requireConfirmed is true when the referenced workspace has confirmed opt-in on.
func requireConfirmed(r ref) *sql.Predicate {
	return sql.P(func(b *sql.Builder) {
		sub := sql.Select(workspace.FieldRequireConfirmedOptIn).From(sql.Table(workspace.Table))
		sub.Where(sql.P(func(pb *sql.Builder) {
			pb.WriteString(sub.C(workspace.FieldID)).WriteString(" = ")
			r.workspace(pb)
		}))
		b.WriteString("COALESCE((").Join(sub).WriteString("), FALSE)")
	})
}
