package bad

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

func update(ctx context.Context, t *ent.Tag, c *ent.OAuthClient) error {
	if err := t.Update().Exec(ctx); err != nil { // want `^rule 0: Update on a loaded Tag writes through the raw client it carries, past the Workspace scope \(ADR 0017\); fix: update it by id through the scoped client: s\.Tag\(\)\.UpdateOneID\(id\)$`
		return err
	}
	return c.Update().Exec(ctx) // want `^rule 0: Update on a loaded OAuthClient`
}

// A method value starts the write where it is called, and is refused where it is taken.
func later(t *ent.Tag) func() *ent.TagUpdateOne {
	return t.Update // want `^rule 0: Update on a loaded Tag`
}
