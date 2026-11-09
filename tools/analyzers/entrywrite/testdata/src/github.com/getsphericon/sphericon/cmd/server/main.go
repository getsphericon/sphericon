package main

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// A command is a driver: it writes nothing through ent.
func run(ctx context.Context, s *ent.Scoped) error {
	return s.Tag().Create().Save(ctx) // want `^rule 5: an entry point writes through ent \(Create on Tag\)`
}

func main() {}
