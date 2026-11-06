// Package testhelper stands in for the test harness: only tests import it, and it is read as one.
package testhelper

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func seed(ctx context.Context, s *ent.Scoped) error {
	return s.Contact().UpdateOneID(1).AddTagIDs(2).Exec(ctx)
}
