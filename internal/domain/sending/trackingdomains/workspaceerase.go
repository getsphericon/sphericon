package trackingdomains

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
)

// Batches of a deleted Workspace's erase (workspacedeletion): the rows with the ids, unchecked.

func EraseTrackingDomainsIn(ctx context.Context, s *ent.Scoped, ids []int64) error {
	_, err := s.TrackingDomain().Delete().Where(trackingdomain.IDIn(ids...)).Exec(ctx)
	return err
}
