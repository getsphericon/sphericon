// Package segmentsaving is the use case that saves a Segment (ADR 0104, ADR 0110): the segments
// module's unchecked save (segmentsunchecked), after the Broadcasts and Automations its event
// tests' filters name are found to be the Workspace's (ADR 0153). The segments module sits in
// contacts, below the Broadcasts and Automations it would ask about (orchestration), so the check
// runs here. An archived Broadcast or Automation is the Workspace's: Archive keeps its row and its
// Events, and neither is ever deleted by its owner (ADR 0100), so the answer cannot change between
// the check and the save.
package segmentsaving

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments/segmentsunchecked"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/messagesenders"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// UseCase saves a Segment. Every call receives the Workspace-scoped client (ADR 0017) built by the
// entry point.
type UseCase struct {
	unchecked *segmentsunchecked.Module
	senders   *messagesenders.Module
}

// New builds the use case over the bus and the segments module; it builds the unchecked save,
// which only a use case may import (ADR 0110), and the senders' check, which holds nothing.
func New(bus *events.Bus, segs *segments.Module) *UseCase {
	return &UseCase{unchecked: segmentsunchecked.New(bus, segs), senders: messagesenders.New()}
}

// Create stores a new Segment (segmentsunchecked.Module.Create), refusing a Condition whose event
// test names a Broadcast or Automation the Workspace does not have (segments.RefuseUnknownSender,
// naming the path of the test). A Condition that does not decode names none, and the save refuses
// it.
func (u *UseCase) Create(ctx context.Context, s *ent.Scoped, in segments.CreateInput) (*ent.Segment, error) {
	if err := u.checkSenders(ctx, s, in.Condition); err != nil {
		return nil, err
	}
	return u.unchecked.Create(ctx, s, in)
}

// Update changes a Segment (segmentsunchecked.Module.Update), refusing a changed Condition as Create
// does.
func (u *UseCase) Update(ctx context.Context, s *ent.Scoped, id int64, in segments.UpdateInput) (*ent.Segment, error) {
	if cond, ok := in.Condition.Get(); ok {
		if err := u.checkSenders(ctx, s, &cond); err != nil {
			return nil, err
		}
	}
	return u.unchecked.Update(ctx, s, id, in)
}

// checkSenders refuses a Condition that names a sender the Workspace does not have; nil for none.
func (u *UseCase) checkSenders(ctx context.Context, s *ent.Scoped, cond *conditiontree.Segment) error {
	if cond == nil {
		return nil
	}
	unknown, err := u.senders.Unknown(ctx, s, condition.NamedSenders(cond.Group))
	if err != nil {
		return err
	}
	if unknown != nil {
		return segments.RefuseUnknownSender(*unknown)
	}
	return nil
}
