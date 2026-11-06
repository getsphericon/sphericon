package invitations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/invitation"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseInvitationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Invitation().Delete().Where(invitation.IDIn(ids...)).Exec(ctx)
	return err
}
