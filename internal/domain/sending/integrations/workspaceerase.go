package integrations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseIntegrationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Integration().Purge().Where(integration.IDIn(ids...)).Exec(ctx)
	return err
}
