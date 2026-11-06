//go:build embed_changelog

package changelog

import _ "embed"

// embedded is the repository's CHANGELOG.md. `mise run build:changelog` copies it next to this
// file before compilation (the copy is gitignored, like the built SPA); building with the
// embed_changelog tag without it fails at compile time, so a release cannot ship without notes.
//
//go:embed CHANGELOG.md
var embedded string

// Embedded is the changelog of this build; empty in a build without the embed_changelog tag.
func Embedded() string { return embedded }
