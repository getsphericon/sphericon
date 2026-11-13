//go:build ignore

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"

	"github.com/getsphericon/sphericon/internal/entgen"
)

// main checks ent/schema against the rules of generation and generates package ent from it. The
// rules, the classification of the entities and the funcs the templates read are internal/entgen's.
func main() {
	cfg := entgen.Config()
	cfg.Target = "."
	cfg.Package = "github.com/getsphericon/sphericon/ent"
	cfg.Features = []gen.Feature{
		gen.FeatureSnapshot,
		gen.FeatureUpsert,
		// Modifier exposes .Modify() on query builders, used by the analytics
		// dashboard for date_trunc day-bucketed engagement aggregates.
		gen.FeatureModifier,
		// Lock exposes .ForUpdate() on query builders: accounts locks the Workspace's
		// owner rows so the Owner invariant holds under concurrent role changes.
		gen.FeatureLock,
		// ExecQuery exposes ExecContext on the client: internal/workspacefeed takes
		// its advisory lock and the next feed position in raw SQL.
		gen.FeatureExecQuery,
		// VersionedMigration generates migrate.NamedDiff, which cmd/db uses to write
		// the next migration straight from the ent schema.
		gen.FeatureVersionedMigration,
	}
	// The check runs on the graph being generated; when it passes, the hook adds ent/template
	// (the scoped client of ADR 0017, the manifest of kinds, the state forms, the bookkeeping
	// columns) with the classification's funcs.
	cfg.Hooks = []gen.Hook{entgen.Hook("template")}
	if err := entc.Generate("./schema", cfg); err != nil {
		log.Fatal("running ent codegen: ", err)
	}
}
