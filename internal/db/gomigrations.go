package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/getsphericon/sphericon/internal/workspaceid"
)

// goMigrations are the migrations written in Go, run by goose in version order among the SQL
// files of migrations/ (a Go migration has no file: Atlas, which only reads the SQL, never sees
// it). Migrate and Status both build their provider from this list, so a status never
// disagrees with what a migrate runs; the FS variants (for tests over a directory of their own)
// take SQL files only. A data step that SQL cannot express without
// database functions (the schema has none) goes here: it must stay fast, and idempotent so a
// re-run after an interrupted Upgrade finishes it (ADR 0033).
func goMigrations() []*goose.Migration {
	return []*goose.Migration{
		// Between 20261020004438_workspace_public_id.sql (the nullable column) and
		// 20261020004440_workspace_public_id_required.sql (NOT NULL and the unique index).
		goose.NewGoMigration(20261020004439, &goose.GoFunc{RunTx: backfillWorkspacePublicIDs}, nil),
	}
}

// backfillWorkspacePublicIDs gives every Workspace without one its Workspace id (ADR 0043).
// It only fills what is empty, so running it again changes nothing.
func backfillWorkspacePublicIDs(ctx context.Context, tx *sql.Tx) error {
	taken, missing, err := workspacePublicIDs(ctx, tx)
	if err != nil {
		return err
	}
	for _, id := range missing {
		publicID := workspaceid.New()
		for taken[publicID] {
			publicID = workspaceid.New()
		}
		taken[publicID] = true
		if _, err := tx.ExecContext(ctx, `UPDATE "workspaces" SET "public_id" = $1 WHERE "id" = $2`, publicID, id); err != nil {
			return fmt.Errorf("workspace %d: %w", id, err)
		}
	}
	return nil
}

// workspacePublicIDs returns the ids already taken and the Workspaces that have none.
func workspacePublicIDs(ctx context.Context, tx *sql.Tx) (taken map[string]bool, missing []int64, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT "id", "public_id" FROM "workspaces"`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()
	taken = map[string]bool{}
	for rows.Next() {
		var id int64
		var publicID sql.NullString
		if err := rows.Scan(&id, &publicID); err != nil {
			return nil, nil, err
		}
		if publicID.Valid {
			taken[publicID.String] = true
		} else {
			missing = append(missing, id)
		}
	}
	return taken, missing, rows.Err()
}
