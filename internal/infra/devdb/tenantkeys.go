package devdb

import (
	"slices"

	atlas "ariga.io/atlas/sql/schema"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/schema"
)

// idColumn is the primary key ent gives every table.
const idColumn = "id"

// tenantKeys makes every reference between two Workspace-owned tables carry the Workspace, so the
// database refuses a row of one Workspace that points at a row of another (ADR 0017): the foreign
// key becomes (workspace_id, x_id) → parent (workspace_id, id), and each parent gets the unique key
// (workspace_id, id) such a key needs. ent declares a foreign key on one column only, so the rule
// is applied here, to the schema the migration is diffed against, from the ent schema itself: a
// new edge between two Workspace-owned entities gets its composite key with no annotation.
//
// A key whose delete action is SET NULL names the one column it clears, SET NULL (x_id), since
// clearing workspace_id too would break its NOT NULL (Postgres 15 and later, postgres.MinMajor).
// Atlas reads that action back as a bare SET NULL, so tenantKeys returns the action each such key
// takes, by its name, for withSetNullColumns to put back on both sides of the diff.
//
// A table is Workspace-owned when the manifest of kinds says its entity is (ent.Kinds: it takes
// WorkspaceMixin), so a table whose workspace_id is declared outside the mixin (the event journal,
// the tracking host change log) keeps its keys as they are, and so do the keys to workspaces.
func tenantKeys(s *atlas.Schema) map[string]atlas.ReferenceOption {
	owned := workspaceOwnedTables()
	workspaceOwned := func(t *atlas.Table) (*atlas.Column, bool) {
		if t == nil || !owned[t.Name] {
			return nil, false
		}
		return t.Column(schema.WorkspaceColumn)
	}
	setNull := map[string]atlas.ReferenceOption{}
	parents := map[*atlas.Table]bool{}
	for _, t := range s.Tables {
		ws, ok := workspaceOwned(t)
		if !ok {
			continue
		}
		for i, fk := range t.ForeignKeys {
			refWS, ok := workspaceOwned(fk.RefTable)
			if !ok || len(fk.Columns) != 1 {
				continue
			}
			parents[fk.RefTable] = true
			column := fk.Columns[0]
			onDelete := fk.OnDelete
			if onDelete == atlas.SetNull {
				onDelete = setNullOf(column.Name)
				setNull[fk.Symbol] = onDelete
			}
			column.ForeignKeys = slices.DeleteFunc(column.ForeignKeys, func(f *atlas.ForeignKey) bool { return f == fk })
			t.ForeignKeys[i] = &atlas.ForeignKey{
				Symbol:     fk.Symbol,
				Table:      t,
				Columns:    []*atlas.Column{ws, column},
				RefTable:   fk.RefTable,
				RefColumns: []*atlas.Column{refWS, fk.RefColumns[0]},
				OnUpdate:   atlas.NoAction,
				OnDelete:   onDelete,
			}
		}
	}
	for p := range parents {
		ws, _ := p.Column(schema.WorkspaceColumn)
		id, _ := p.Column(idColumn)
		p.AddIndexes(atlas.NewUniqueIndex(p.Name+"_workspace_id_id").AddColumns(ws, id))
	}
	return setNull
}

// setNullOf is the delete action that clears only column.
func setNullOf(column string) atlas.ReferenceOption {
	return atlas.ReferenceOption(string(atlas.SetNull) + " (" + column + ")")
}

// withSetNullColumns puts back on s the delete action each key named in actions takes, where Atlas
// read it as a bare SET NULL.
func withSetNullColumns(s *atlas.Schema, actions map[string]atlas.ReferenceOption) {
	for _, t := range s.Tables {
		for _, fk := range t.ForeignKeys {
			if a, ok := actions[fk.Symbol]; ok && fk.OnDelete == atlas.SetNull {
				fk.OnDelete = a
			}
		}
	}
}

// workspaceOwnedTables are the tables of the Workspace-owned entities, from the manifest of kinds.
func workspaceOwnedTables() map[string]bool {
	owned := map[string]bool{}
	for _, k := range ent.Kinds {
		if k.WorkspaceOwned {
			owned[k.Table] = true
		}
	}
	return owned
}
