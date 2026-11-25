// Package segmentsaving is the use case that saves a Segment (ADR 0104): the segments module's
// Create and Update, after the Broadcasts and Automations its event tests' filters name are found
// to be the Workspace's (ADR 0153). The segments module sits in contacts, below the Broadcasts and
// Automations it would ask (orchestration), so the check runs here. An archived Broadcast or
// Automation is the Workspace's: Archive keeps its row and its Events, and neither is ever deleted
// by its owner (ADR 0100), so the answer cannot change between the check and the save.
package segmentsaving

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/messagesenders"
)

// UseCase saves a Segment. Every call receives the Workspace-scoped client (ADR 0017) built by the
// entry point.
type UseCase struct {
	segments *segments.Module
	senders  *messagesenders.Module
}

// New builds the use case over the segments module; it builds the senders' check, which holds
// nothing.
func New(segs *segments.Module) *UseCase {
	return &UseCase{segments: segs, senders: messagesenders.New()}
}

// Create stores a new Segment, as segments.Module.Create does, refusing a Condition whose event test
// names a Broadcast or Automation the Workspace does not have (messagesenders.ErrUnknown, naming
// the path of the test). A Condition that does not decode names none, and the module refuses it.
func (u *UseCase) Create(ctx context.Context, s *ent.Scoped, in segments.CreateInput) (*ent.Segment, error) {
	if in.Condition != nil {
		if err := u.senders.Require(ctx, s, condition.NamedSenders(in.Condition.Group)); err != nil {
			return nil, err
		}
	}
	return u.segments.Create(ctx, s, in)
}

// Update changes a Segment, as segments.Module.Update does, refusing a changed Condition as Create
// does.
func (u *UseCase) Update(ctx context.Context, s *ent.Scoped, id int64, in segments.UpdateInput) (*ent.Segment, error) {
	if cond, ok := in.Condition.Get(); ok {
		if err := u.senders.Require(ctx, s, condition.NamedSenders(cond.Group)); err != nil {
			return nil, err
		}
	}
	return u.segments.Update(ctx, s, id, in)
}
