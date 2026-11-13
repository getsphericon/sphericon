package automationsunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationfigure"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// AddPruned counts the Automation messages message retention deletes into their Automations'
// figures, one row per step position (ADR 0143), in the transaction that deletes them: the use
// case internal/usecase/messageretention runs it. It counts as the report does, sent and skipped by
// step and the delivery stamps whatever the status, so the report reads the same figures after the
// prune as before. Run alone, it would count messages twice.
func AddPruned(ctx context.Context, s *ent.Scoped, msgs []*ent.OutboundMessage) error {
	type key struct {
		automation int64
		step       int
	}
	byStep := map[key]*counts{}
	var order []key
	for _, m := range msgs {
		if m.AutomationID == nil || m.AutomationStep == nil {
			continue // the provenance check makes both required on an Automation message
		}
		k := key{automation: *m.AutomationID, step: *m.AutomationStep}
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
			SetAutomationID(k.automation).SetStep(k.step).
			SetSent(c.sent).SetSkipped(c.skipped).SetDelivered(c.delivered).
			SetBounced(c.bounced).SetComplained(c.complained).SetCaptured(c.captured).
			OnConflictColumns(automationfigure.FieldWorkspaceID, automationfigure.FieldAutomationID, automationfigure.FieldStep).
			Update(func(u *ent.AutomationFigureScopedUpsert) {
				u.AddSent(c.sent).AddSkipped(c.skipped).AddDelivered(c.delivered).
					AddBounced(c.bounced).AddComplained(c.complained).AddCaptured(c.captured).
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
	sent, skipped, delivered, bounced, complained, captured int
}

func (c *counts) add(m *ent.OutboundMessage) {
	switch m.Status {
	case outboundmessage.StatusSent:
		c.sent++
	case outboundmessage.StatusSkipped:
		c.skipped++
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
