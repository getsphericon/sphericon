package integrations

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
)

// EraseIntegrationsIn removes the scoped Workspace's Integrations with the ids, discarded or not,
// inside the transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseIntegrationsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.Integration().Purge().Where(integration.IDIn(ids...)).Exec(ctx)
	return err
}
