package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func run(ctx context.Context, client *ent.Client, s *ent.Scoped) {
	_ = client.Tag.Create().SetName("x").Save(ctx) // want `^rule 5: an entry point writes through ent \(Create\).*; fix: move the write into the domain module`
	_ = client.Tag.UpdateOneID(1).Exec(ctx)        // want `^rule 5: .*\(UpdateOneID\)`
	_ = client.Tag.DeleteOneID(1).Exec(ctx)        // want `^rule 5: .*\(DeleteOneID\)`
	_ = s.Tag().Create().SetName("x").Save(ctx)    // want `^rule 5: .*\(Create\)`
	_ = s.Tag().Update().Exec(ctx)                 // want `^rule 5: .*\(Update\)`
	_ = s.Tag().CreateBulk().Save(ctx)             // want `^rule 5: .*\(CreateBulk\)`
}
