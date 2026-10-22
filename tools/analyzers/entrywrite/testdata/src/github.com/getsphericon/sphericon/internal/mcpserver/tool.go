package mcpserver

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func tool(ctx context.Context, s *ent.Scoped) error {
	return s.Tag().Create().Save(ctx) // want `^rule 5: .*\(Create on Tag\)`
}
