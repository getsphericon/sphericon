// Package callbackinterpretation is the Callback interpretation (GLOSSARY), behind the Ingest
// hook: it reads the facts a Preset parsed from an Integration's callbacks for what they mean (ADR
// 0076): the Preset parses, the core interprets. Each Channel has one Interpreter, picked by the
// Integration's Channel in interpreterFor, the one place a Channel plugs in; the facts are written
// in one transaction before the Ingest hook answers 2xx (ADR 0037). It is a domain module of the
// sending context without a table (ADR 0112): the Outbound messages it stamps and the Bindings it
// changes go through their modules (outboundmessages, bindings), and the rest is published as
// System events on the bus (a delivery, a stop or a start observed in the Channel, a received
// message, a number that reaches no handset), whose consumers write what follows from them; a
// notice that came before its message is queued as a job to be read again (LateNoticeArgs).
package callbackinterpretation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Interpreter reads the facts of one Channel for what they mean there: a Delivery notice
// stamps and suppresses on email, a `/start <token>` binds a Telegram chat. Every call runs in
// the request's one transaction (Tx), so whatever it writes, and every System event it
// publishes, commits together or not at all.
type Interpreter interface {
	DeliveryNotice(ctx context.Context, tx Tx, n preset.DeliveryNotice) error
	InboundMessage(ctx context.Context, tx Tx, m preset.InboundMessage) error
	ReachabilityChange(ctx context.Context, tx Tx, c preset.ReachabilityChange) error
}

// ignore is the Interpreter that records nothing. A Channel's Interpreter embeds it and
// implements only the facts it reads; a Channel that reads none is ignore itself.
type ignore struct{}

// DeliveryNotice records nothing.
func (ignore) DeliveryNotice(context.Context, Tx, preset.DeliveryNotice) error { return nil }

// InboundMessage records nothing.
func (ignore) InboundMessage(context.Context, Tx, preset.InboundMessage) error { return nil }

// ReachabilityChange records nothing.
func (ignore) ReachabilityChange(context.Context, Tx, preset.ReachabilityChange) error { return nil }

// Tx is what one request's facts are interpreted with: the transaction, the Integration the
// callback came to, the Preset that parsed it, and the hook's clock.
type Tx struct {
	// Scoped is the Integration's Workspace, inside the transaction.
	Scoped *ent.Scoped
	// Pub publishes System events in the transaction.
	Pub         events.Publisher
	Integration *ent.Integration
	// Preset is the name of the Preset that parsed the request: the provider an email delivery
	// Event names.
	Preset string
	Now    time.Time

	after *[]func(context.Context)
	// queue queues a job in the transaction: a notice deferred until its message is committed.
	queue events.Enqueuer
	// attempt is how many times the facts were read before: 0 at the hook, n for a deferred
	// notice's nth reading again.
	attempt int
}

// After queues work for after the request's transaction commits, before the hook answers: a
// provider's acknowledgement (Telegram's answerCallbackQuery) that must not happen for facts that
// were rolled back. It is best effort: it cannot fail the request, which is already recorded, so
// it logs its own errors.
func (tx Tx) After(f func(context.Context)) { *tx.after = append(*tx.after, f) }

// DedupKey is the key a fact's writes deduplicate on: the provider's event id, which is unique
// only per provider account, prefixed with the Channel and the Integration (on Telegram
// `telegram:<integration id>:<update_id>`, since an update_id is per bot).
func (tx Tx) DedupKey(eventID string) string {
	if eventID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d:%s", tx.Integration.Channel, tx.Integration.ID, eventID)
}

// answerTimeout bounds one acknowledgement to a provider after the commit.
const answerTimeout = 5 * time.Second

// Module interprets the facts of every Ingest hook request.
type Module struct {
	// client finds the Integration an ingest key names and scopes to its Workspace, before any
	// scope exists (docs/architecture/workspace-scoping.md, Workspace scoping).
	client *ent.Client
	bus    *events.Bus
	// integ is what the hook reads of an Integration besides its row: its Preset with the Preset's
	// params, its hook URL, the confirmation of a subscription its provider asks for, and the bot
	// of a Telegram Integration.
	integ *integrations.Module
	now   func() time.Time
}

// New builds the module on the raw client an ingest key is resolved with, the bus the facts'
// System events are published through, the Integrations module and the Instance clock the hook
// reads.
func New(client *ent.Client, bus *events.Bus, integ *integrations.Module, now func() time.Time) *Module {
	return &Module{client: client, bus: bus, integ: integ, now: now}
}

// InterpretInput is one Ingest hook request: the Integration the callback came to, the name of
// the Preset that parsed it and the facts the Preset read.
type InterpretInput struct {
	Integration *ent.Integration
	Preset      string
	Result      preset.Result
}

// interpreterFor is the Interpreter of a Channel. A new Channel value fails the exhaustive
// lint here until its Channel is given one: its own, or ignore while it reads nothing.
func (m *Module) interpreterFor(ch channels.Channel) Interpreter {
	switch ch {
	case channels.Email:
		return email{}
	case channels.Telegram:
		return telegram{integ: m.integ}
	case channels.SMS:
		return sms{}
	}
	return ignore{}
}

// Interpret writes what the facts of one request mean on the Integration's Channel, in one
// transaction of s's Workspace. A Delivery notice or an Inbound message that names another
// Channel than the Integration's (an Integration serves one Channel, ADR 0074) is the Preset's
// mistake: it is dropped and logged, so the rest of the request still counts and the provider
// does not redeliver it forever.
func (m *Module) Interpret(ctx context.Context, s *ent.Scoped, in InterpretInput) error {
	return m.interpret(ctx, s, in.Integration, in.Preset, in.Result, m.now(), 0)
}

// interpret is Interpret for the facts read for the attempt-th time before (InterpretLate).
func (m *Module) interpret(ctx context.Context, s *ent.Scoped, row *ent.Integration, presetName string, res preset.Result, now time.Time, attempt int) error {
	if res.Empty() {
		return nil
	}
	interp := m.interpreterFor(row.Channel)
	var after []func(context.Context)
	err := m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher, q events.Enqueuer) error {
		after = after[:0]
		tx := Tx{Scoped: ts, Pub: pub, Integration: row, Preset: presetName, Now: now, after: &after, queue: q, attempt: attempt}
		for i := range res.Notices {
			n := &res.Notices[i]
			if string(n.Channel) != string(row.Channel) {
				slog.WarnContext(ctx, "ingest: a Delivery notice of another Channel than its Integration's is dropped",
					"integration_id", row.ID, "preset", presetName, "channel", n.Channel, "event_id", n.ID)
				continue
			}
			if err := interp.DeliveryNotice(ctx, tx, *n); err != nil {
				return err
			}
		}
		for i := range res.InboundMessages {
			msg := &res.InboundMessages[i]
			if string(msg.Channel) != string(row.Channel) {
				slog.WarnContext(ctx, "ingest: an Inbound message of another Channel than its Integration's is dropped",
					"integration_id", row.ID, "preset", presetName, "channel", msg.Channel, "event_id", msg.ID)
				continue
			}
			if err := interp.InboundMessage(ctx, tx, *msg); err != nil {
				return err
			}
		}
		for _, c := range res.ReachabilityChanges {
			if err := interp.ReachabilityChange(ctx, tx, c); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, f := range after {
		answerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), answerTimeout)
		f(answerCtx)
		cancel()
	}
	return nil
}
