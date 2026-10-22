package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func run(ctx context.Context, client *ent.Client, s *ent.Scoped) {
	_ = client.Tag.Create().SetName("x").Save(ctx) // want `^rule 5: an entry point writes through ent \(Create on Tag\).*; fix: move the write into the domain module`
	_ = client.Tag.UpdateOneID(1).Exec(ctx)        // want `^rule 5: .*\(UpdateOneID on Tag\)`
	_ = client.Tag.DeleteOneID(1).Exec(ctx)        // want `^rule 5: .*\(DeleteOneID on Tag\)`
	_ = s.Tag().Create().SetName("x").Save(ctx)    // want `^rule 5: .*\(Create on Tag\)`
	_ = s.Tag().Update().Exec(ctx)                 // want `^rule 5: .*\(Update on Tag\)`
	_ = s.Tag().CreateBulk().Save(ctx)             // want `^rule 5: .*\(CreateBulk on Tag\)`
}
