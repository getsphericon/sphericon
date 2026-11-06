// Package tagdeletion is the use case that deletes a Tag from the catalogue (ADR 0102, ADR 0049).
// A Tag is named by the modules above the tags module: a Segment's Condition, a step of a kept
// Automation, a kept Rule and the Condition an armed Broadcast copied from its Segment. Deleting it
// asks each of them whether it still names the Tag, and refuses with an *inuse.Error naming them if
// one does, because each would silently stop matching or applying. The tags module knows none of
// them, so the check lives here and calls the module's unchecked removal, tags.DeleteIn, which
// easement forbids every other package to call.
package tagdeletion

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/inuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// UseCase deletes a Tag. Every call receives the Workspace-scoped client (ADR 0017) built by the
// entry point; the use case opens the transaction the modules it calls join.
type UseCase struct {
	bus *events.Bus
}

// New builds the Tag deletion use case.
func New(bus *events.Bus) *UseCase { return &UseCase{bus: bus} }

// Delete removes a Tag from the catalogue, and from every Contact that has it, unless a Segment
// rule or a step of a kept Automation's live version or draft names it: then it returns an
// *inuse.Error naming those Segments and Automations (ADR 0049, ADR 0065), because both name the
// Tag and would silently stop matching or applying.
// The Condition an armed Broadcast copied from its Segment counts too (ADR 0065). A missing Tag is
// tags.ErrNotFound. The check and the delete run in one transaction under the Workspace's lock (ADR
// 0099), which saving a Segment, a Rule or an Automation takes too, so none of them names the Tag
// between the check and the delete.
func (u *UseCase) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return u.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		t, err := tags.Get(ctx, ts, id)
		if err != nil {
			return err
		}
		users, err := tagUsers(ctx, ts, t.Name)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return &inuse.Error{What: inuse.SubjectTag, Users: users}
		}
		return tags.DeleteIn(ctx, ts, id)
	})
}

// tagUsers is what names the Tag: Segments, kept Automations, kept Rules and armed Broadcasts.
func tagUsers(ctx context.Context, s *ent.Scoped, name string) ([]inuse.User, error) {
	var users []inuse.User
	segs, err := segments.UsingTag(ctx, s, name)
	if err != nil {
		return nil, err
	}
	for _, seg := range segs {
		users = append(users, inuse.User{Kind: inuse.Segment, ID: seg.ID, Name: seg.Name})
	}
	autos, err := automations.UsingTag(ctx, s, name)
	if err != nil {
		return nil, err
	}
	for _, a := range autos {
		users = append(users, inuse.User{Kind: inuse.Automation, ID: a.ID, Name: a.Name})
	}
	live, err := rules.UsingTag(ctx, s, name)
	if err != nil {
		return nil, err
	}
	users = append(users, inuse.Rules(live)...)
	armed, err := broadcasts.UsingTag(ctx, s, name)
	if err != nil {
		return nil, err
	}
	return append(users, inuse.Broadcasts(armed)...), nil
}
