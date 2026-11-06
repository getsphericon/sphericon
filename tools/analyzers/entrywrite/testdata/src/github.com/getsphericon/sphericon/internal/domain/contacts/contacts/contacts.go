package contacts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The owner of Contact writes it, its join rows with Tags included.
func tag(ctx context.Context, s *ent.Scoped) error {
	return s.Contact().UpdateOneID(1).AddTagIDs(2).Exec(ctx)
}
