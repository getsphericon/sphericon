package automationsunchecked

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationfigure"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// AddPruned counts the Automation messages message retention deletes into their Automations'
// figures, one row per version and Step id (ADR 0143, ADR 0122), in the transaction that deletes them: the use
// case internal/usecase/messageretention runs it. It counts as the report does, sent and skipped by
// step, the skipped by their Skipped reason (ADR 0178), the sent a Sending window judged by a single
// zone (ADR 0134), and the delivery stamps whatever the status, so the report reads the same figures after the
// prune as before. Run alone, it would count messages twice.
func (m *Module) AddPruned(ctx context.Context, s *ent.Scoped, msgs []*ent.OutboundMessage) error {
	type key struct {
		automation, version int64
		step                automationstep.ID
	}
	byStep := map[key]*counts{}
	var order []key
	for _, m := range msgs {
		if m.AutomationID == nil || m.AutomationVersionID == nil || m.AutomationStepID == nil {
			continue // the provenance check makes them required on an Automation message
		}
		k := key{automation: *m.AutomationID, version: *m.AutomationVersionID, step: *m.AutomationStepID}
		c := byStep[k]
		if c == nil {
			c = &counts{}
			byStep[k] = c
			order = append(order, k)
		}
		c.add(m)
	}
	for _, k := range order {
		c := byStep[k]
		err := s.AutomationFigure().Create().
			SetAutomationID(k.automation).SetVersionID(k.version).SetStepID(k.step).
			SetSent(c.sent).SetSkipped(c.skipped).SetDelivered(c.delivered).
			SetBounced(c.bounced).SetComplained(c.complained).SetCaptured(c.captured).
			SetSkippedSuppressed(c.skippedBy[skipreason.Suppressed]).
			SetSkippedUnsubscribedEverything(c.skippedBy[skipreason.UnsubscribedEverything]).
			SetSkippedUnsubscribedSource(c.skippedBy[skipreason.UnsubscribedSource]).
			SetSkippedUnconfirmed(c.skippedBy[skipreason.Unconfirmed]).
			SetSkippedNoDestination(c.skippedBy[skipreason.NoDestination]).
			SetSkippedContactErased(c.skippedBy[skipreason.ContactErased]).
			SetSkippedNoLocalTime(c.skippedBy[skipreason.NoLocalTime]).
			SetSkippedFrequencyCap(c.skippedBy[skipreason.FrequencyCap]).
			SetSingleZone(c.singleZone).
			OnConflictColumns(automationfigure.FieldWorkspaceID, automationfigure.FieldAutomationID, automationfigure.FieldVersionID, automationfigure.FieldStepID).
			Update(func(u *ent.AutomationFigureScopedUpsert) {
				u.AddSent(c.sent).AddSkipped(c.skipped).AddDelivered(c.delivered).
					AddBounced(c.bounced).AddComplained(c.complained).AddCaptured(c.captured).
					AddSkippedSuppressed(c.skippedBy[skipreason.Suppressed]).
					AddSkippedUnsubscribedEverything(c.skippedBy[skipreason.UnsubscribedEverything]).
					AddSkippedUnsubscribedSource(c.skippedBy[skipreason.UnsubscribedSource]).
					AddSkippedUnconfirmed(c.skippedBy[skipreason.Unconfirmed]).
					AddSkippedNoDestination(c.skippedBy[skipreason.NoDestination]).
					AddSkippedContactErased(c.skippedBy[skipreason.ContactErased]).
					AddSkippedNoLocalTime(c.skippedBy[skipreason.NoLocalTime]).
					AddSkippedFrequencyCap(c.skippedBy[skipreason.FrequencyCap]).
					AddSingleZone(c.singleZone).
					UpdateUpdatedAt()
			}).
			Exec(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// counts is what a batch of messages adds to one step's figures.
type counts struct {
	sent, skipped, delivered, bounced, complained, captured, singleZone int
	skippedBy                                                           map[skipreason.Reason]int
}

func (c *counts) add(m *ent.OutboundMessage) {
	switch m.Status {
	case outboundmessage.StatusSent:
		c.sent++
		if m.WindowZones == outboundmessage.WindowZonesOneZone {
			c.singleZone++
		}
	case outboundmessage.StatusSkipped:
		c.skipped++
		if c.skippedBy == nil {
			c.skippedBy = map[skipreason.Reason]int{}
		}
		// The outcome check sets a reason on every skipped message.
		c.skippedBy[lo.FromPtr(m.SkippedReason)]++
	case outboundmessage.StatusFailed, outboundmessage.StatusPending:
	}
	if m.DeliveredAt != nil {
		c.delivered++
	}
	if m.BouncedAt != nil {
		c.bounced++
	}
	if m.ComplainedAt != nil {
		c.complained++
	}
	if m.Captured {
		c.captured++
	}
}
