package sendingdomains

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseSendingDomainsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.SendingDomain().Purge().Where(sendingdomain.IDIn(ids...)).Exec(ctx)
	return err
}
