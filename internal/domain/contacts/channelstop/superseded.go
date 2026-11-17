package channelstop

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
)

// publishSuperseded records e, an observation that changed nothing (ADR 0154), for each of
// contactIDs still in the Workspace: the place it takes in their consent history. Its source id
// derives from the observation's, per Contact, so a redelivery records nothing twice; an
// observation with none (one recorded by hand) records it each time.
func (m *Module) publishSuperseded(ctx context.Context, s *ent.Scoped, contactIDs []int64, e *events.ConsentSuperseded) error {
	for _, id := range contactIDs {
		c, err := s.Contact().Get(ctx, id)
		if ent.IsNotFound(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read contact: %w", err)
		}
		one := *e
		one.WorkspaceID, one.ContactID, one.Email = s.WorkspaceID(), c.ID, lo.FromPtr(c.Email)
		if e.DedupID != "" {
			one.DedupID = fmt.Sprintf("%s/superseded/%d", e.DedupID, c.ID)
		}
		if err := m.bus.PublishIn(ctx, s, &one); err != nil {
			return err
		}
	}
	return nil
}

// publishSMSSuperseded records e, an SMS observation of number that changed nothing, for the
// Contact whose phone the number is; a number no Contact has records nothing.
func (m *Module) publishSMSSuperseded(ctx context.Context, s *ent.Scoped, number string, e *events.ConsentSuperseded) error {
	id, err := s.Contact().Query().Where(contact.Phone(number)).OnlyID(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find the number's contact: %w", err)
	}
	e.Channel, e.Phone = string(channels.SMS), number
	return m.publishSuperseded(ctx, s, []int64{id}, e)
}
