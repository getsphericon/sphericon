package commandline

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// The command line's driver package writes nothing through ent either.
func rename(ctx context.Context, s *ent.Scoped) error {
	return s.Tag().Update().Exec(ctx) // want `^rule 5: an entry point writes through ent \(Update on Tag\)`
}
