// Package jobs stands in for ee/jobs: the Enterprise Edition's job runner, an entry point like the
// core's.
package jobs

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func run(ctx context.Context, client *ent.Client) {
	_ = client.Tag.Create().SetName("x").Save(ctx) // want `^rule 5: an entry point writes through ent \(Create on Tag\).*; fix: move the write into the domain module`
	_, _ = client.Tag.Query().All(ctx)             // a job may read
}
