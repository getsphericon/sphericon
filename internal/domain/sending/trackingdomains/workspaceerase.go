package trackingdomains

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
)

// EraseTrackingDomainsIn removes the scoped Workspace's Tracking domains with the ids, inside the
// transaction s is bound to: one batch of the erase of a deleted Workspace
// (internal/usecase/workspacedeletion, ADR 0043), which lists the ids and checks nothing else.
func EraseTrackingDomainsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.TrackingDomain().Delete().Where(trackingdomain.IDIn(ids...)).Exec(ctx)
	return err
}
