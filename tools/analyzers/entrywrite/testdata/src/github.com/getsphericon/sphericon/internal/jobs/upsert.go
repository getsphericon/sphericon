package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func upsert(ctx context.Context, client *ent.Client) {
	_ = client.Tag.Create().OnConflictColumns("name").Save(ctx) // want `^rule 5: .*\(Create on Tag\)`
	b := client.Tag.Create()                                    // want `^rule 5: .*\(Create on Tag\)`
	_ = b.OnConflict().Save(ctx)                                // want `^rule 5: .*\(OnConflict on Tag\)`
}
