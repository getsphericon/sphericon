package accounts

import (
	"context"
	"fmt"

	"github.com/gosimple/slug"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/workspace"
)

// Slugify converts an arbitrary string into a URL-safe slug: lowercased, with
// Unicode transliterated to ASCII (e.g. "Привет Мир" -> "privet-mir") and runs
// of other characters collapsed to single hyphens. Returns "" when the input
// has no usable characters (callers should fall back). Thin wrapper over
// gosimple/slug so we don't maintain transliteration tables by hand.
func Slugify(s string) string {
	return slug.Make(s)
}

// WorkspaceIDBySlug resolves a Workspace slug to its id (staff member tooling addresses
// Workspaces by slug).
func WorkspaceIDBySlug(ctx context.Context, client *ent.Client, slug string) (int64, error) {
	id, err := client.Workspace.Query().Where(workspace.Slug(slug)).OnlyID(ctx)
	if err != nil {
		return 0, fmt.Errorf("workspace %q: %w", slug, err)
	}
	return id, nil
}
