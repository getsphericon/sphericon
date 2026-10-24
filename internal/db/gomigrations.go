package db

import "github.com/pressly/goose/v3"

// goMigrations are the migrations written in Go, run by goose in version order among the SQL
// files of migrations/ (a Go migration has no file, so the ent diff never sees it). Migrate and Status both build their provider from this list, so a status never
// disagrees with what a migrate runs; the FS variants (for tests over a directory of their own)
// take SQL files only. A data step that SQL cannot express without
// database functions (the schema has none) goes here: it must stay fast, and idempotent so a
// re-run after an interrupted Upgrade finishes it (ADR 0033).
func goMigrations() []*goose.Migration {
	return nil
}
