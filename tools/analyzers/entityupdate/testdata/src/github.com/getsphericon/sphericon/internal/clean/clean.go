package clean

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The scoped client, the raw client and an upsert's Update are no entity's.
func update(ctx context.Context, s *ent.TagScoped, c *ent.TagClient, u *ent.TagUpsertOne) error {
	if err := s.UpdateOneID(1).Exec(ctx); err != nil {
		return err
	}
	if err := s.Update().Exec(ctx); err != nil {
		return err
	}
	u.Update(func() {})
	return c.Update().Exec(ctx)
}
