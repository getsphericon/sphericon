// This file exists only to embed the
// goose migration files (which live in ./migrations at the repo
// root) so the single binary can apply them itself (goose), with no migration
// tool on the host. The embed directive must live at or above the migrations directory in
// the source tree — go:embed cannot reference parent paths — hence this file
// sits at the module root.

package sphericon

import "embed"

// MigrationsFS holds the committed migration files (goose format, diffed
// from the ent schema). Consumed by cmd/server. Written by `mise run db:generate`, never by hand.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
