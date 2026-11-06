// Package e2e stands in for the end-to-end suite: test code behind its build tag, read as one.
package e2e

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func seed(ctx context.Context, s *ent.Scoped) error {
	return s.Contact().UpdateOneID(1).AddTagIDs(2).Exec(ctx)
}
