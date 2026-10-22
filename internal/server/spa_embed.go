//go:build embed_spa

package server

import (
	"embed"
	"io/fs"
)

// embeddedSPA holds the built Vite frontend. `mise run build:spa` runs `pnpm build`
// and copies dist/ into assets/spa/ before compilation. The directory is
// gitignored and only populated for release builds — building with the
// embed_spa tag against an empty assets/spa fails loudly at compile time,
// which is the intended guard against shipping a binary with no UI.
//
//go:embed all:assets/spa
var embeddedSPA embed.FS

func spaFS() (fs.FS, bool) {
	sub, err := fs.Sub(embeddedSPA, "assets/spa")
	if err != nil {
		return nil, false
	}
	return sub, true
}
