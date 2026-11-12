package devdb

import (
	"slices"

	atlas "ariga.io/atlas/sql/schema"

	"github.com/getsphericon/sphericon/ent/workspace"
)

// workspaceColumn is the column every Workspace-owned table carries (the workspace mixin).
const workspaceColumn = "workspace_id"

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
// A table is Workspace-owned when its workspace_id is NOT NULL, so a table whose rows may belong to
// no Workspace (the event journal) keeps its keys as they are, and so do the keys to workspaces.
func tenantKeys(s *atlas.Schema) map[string]atlas.ReferenceOption {
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
		ws, _ := p.Column(workspaceColumn)
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

// workspaceOwned is the workspace_id column of t when t is Workspace-owned: it has one and it is
// NOT NULL. The workspaces table itself is not.
func workspaceOwned(t *atlas.Table) (*atlas.Column, bool) {
	if t == nil || t.Name == workspace.Table {
		return nil, false
	}
	c, ok := t.Column(workspaceColumn)
	if !ok || c.Type == nil || c.Type.Null {
		return nil, false
	}
	return c, true
}
