// Package ingest reads the facts a Preset parsed from an Integration's callbacks for what they
// mean (ADR 0076): the Preset parses, the core interprets. Each Channel has one Interpreter,
// picked by the Integration's Channel in interpreterFor, the one place a Channel plugs in; the
// facts are written in one transaction before the Ingest hook answers 2xx (ADR 0037).
package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/preset"
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

// Ignore is the Interpreter that records nothing. A Channel's Interpreter embeds it and
// implements only the facts it reads; a Channel that reads none is Ignore itself.
type Ignore struct{}

// DeliveryNotice records nothing.
func (Ignore) DeliveryNotice(context.Context, Tx, preset.DeliveryNotice) error { return nil }

// InboundMessage records nothing.
func (Ignore) InboundMessage(context.Context, Tx, preset.InboundMessage) error { return nil }

// ReachabilityChange records nothing.
func (Ignore) ReachabilityChange(context.Context, Tx, preset.ReachabilityChange) error { return nil }

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
}

// DedupKey is the key a fact's writes deduplicate on: the provider's event id, which is unique
// only per provider account, prefixed with the Channel and the Integration (on Telegram
// `telegram:<integration id>:<update_id>`, since an update_id is per bot).
func (tx Tx) DedupKey(eventID string) string {
	return fmt.Sprintf("%s:%d:%s", tx.Integration.Channel, tx.Integration.ID, eventID)
}

// Module interprets the facts of every Ingest hook request.
type Module struct {
	bus *events.Bus
}

// New builds the module on the bus the facts' System events are published through.
func New(bus *events.Bus) *Module { return &Module{bus: bus} }

// interpreterFor is the Interpreter of a Channel. A new Channel value fails the exhaustive
// lint here until its Channel is given one: its own, or Ignore while it reads nothing.
func (m *Module) interpreterFor(ch integration.Channel) Interpreter {
	switch ch {
	case integration.ChannelEmail:
		return email{}
	case integration.ChannelTelegram:
		return Ignore{}
	case integration.ChannelSms:
		return Ignore{}
	}
	return Ignore{}
}

// Interpret writes what the facts of one request mean on the Integration's Channel, in one
// transaction of s's Workspace. An Inbound message that names another Channel than the
// Integration's (an Integration serves one Channel, ADR 0074) is the Preset's mistake: it is
// dropped and logged, so the rest of the request still counts and the provider does not
// redeliver it forever.
func (m *Module) Interpret(ctx context.Context, s *ent.Scoped, row *ent.Integration, presetName string, res preset.Result, now time.Time) error {
	if res.Empty() {
		return nil
	}
	interp := m.interpreterFor(row.Channel)
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, pub events.Publisher) error {
		tx := Tx{Scoped: ts, Pub: pub, Integration: row, Preset: presetName, Now: now}
		for _, n := range res.Notices {
			if err := interp.DeliveryNotice(ctx, tx, n); err != nil {
				return err
			}
		}
		for _, msg := range res.InboundMessages {
			if string(msg.Channel) != string(row.Channel) {
				logging.FromContext(ctx).Warn("ingest: an Inbound message of another Channel than its Integration's is dropped",
					"integration_id", row.ID, "preset", presetName, "channel", msg.Channel, "event_id", msg.ID)
				continue
			}
			if err := interp.InboundMessage(ctx, tx, msg); err != nil {
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
}
