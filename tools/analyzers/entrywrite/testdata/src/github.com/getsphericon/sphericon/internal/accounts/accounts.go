package accounts

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// A domain module writes through ent: it is not an entry point.
func create(ctx context.Context, client *ent.Client, s *ent.Scoped) error {
	if err := client.Tag.Create().Save(ctx); err != nil {
		return err
	}
	return s.Tag().Update().Exec(ctx)
}
