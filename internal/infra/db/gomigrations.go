package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// goMigrations are the migrations written in Go, run by goose in version order among the SQL
// files of migrations/ (a Go migration has no file, so the ent diff never sees it). Migrate and Status both build their provider from this list, so a status never
// disagrees with what a migrate runs; the FS variants (for tests over a directory of their own)
// take SQL files only. A data step that SQL cannot express without
// database functions (the schema has none) goes here: it must stay fast, and idempotent so a
// re-run after an interrupted Upgrade finishes it (ADR 0033).
func goMigrations() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(PhonesToE164Version, &goose.GoFunc{RunTx: phonesToE164}, nil),
	}
}

// PhonesToE164Version is the version of the Go migration that rewrites Contact phones to E.164.
const PhonesToE164Version = 20261103233023

// storedPhone is one Contact's phone as the migration reads it.
type storedPhone struct {
	id, workspaceID int64
	phone           string
	country         sql.NullString
}

// phonesToE164 rewrites every Contact's phone to E.164 (ADR 0075), a number without a country
// code read with the Contact's country, and clears a phone that does not parse (greenfield: no
// compatibility path). When two Contacts of a Workspace normalize to one number, the one that
// already stores it in E.164 keeps it, and otherwise the oldest (lowest id); the others' phone is
// cleared, so the unique (phone, workspace_id) index never refuses the rewrite. Losers are cleared
// before winners are written for the same reason. A second run changes nothing.
func phonesToE164(ctx context.Context, tx *sql.Tx) error {
	stored, err := storedPhones(ctx, tx)
	if err != nil {
		return err
	}
	target := make(map[int64]string, len(stored))
	for _, s := range stored {
		if v, err := phone.Normalize(s.phone, s.country.String); err == nil {
			target[s.id] = v
		}
	}
	type key struct {
		workspaceID int64
		phone       string
	}
	keeper := map[key]int64{}
	for _, s := range stored {
		if t, ok := target[s.id]; ok && t == s.phone {
			keeper[key{s.workspaceID, t}] = s.id
		}
	}
	for _, s := range stored { // in id order: the oldest claims a number nobody stores yet
		if t, ok := target[s.id]; ok {
			if _, taken := keeper[key{s.workspaceID, t}]; !taken {
				keeper[key{s.workspaceID, t}] = s.id
			}
		}
	}
	var cleared []int64
	rewritten := map[int64]string{}
	for _, s := range stored {
		t, ok := target[s.id]
		switch {
		case !ok || keeper[key{s.workspaceID, t}] != s.id:
			cleared = append(cleared, s.id)
		case t != s.phone:
			rewritten[s.id] = t
		}
	}
	for _, id := range cleared {
		// raw sql: a Go migration runs on the schema as it was then (frozen); the current ent
		// entities describe a later one.
		if _, err := tx.ExecContext(ctx, `UPDATE "contacts" SET "phone" = NULL WHERE "id" = $1`, id); err != nil {
			return fmt.Errorf("clear the phone of contact %d: %w", id, err)
		}
	}
	for id, p := range rewritten {
		// raw sql: a Go migration runs on the frozen schema, see above.
		if _, err := tx.ExecContext(ctx, `UPDATE "contacts" SET "phone" = $1 WHERE "id" = $2`, p, id); err != nil {
			return fmt.Errorf("rewrite the phone of contact %d: %w", id, err)
		}
	}
	return nil
}

// storedPhones is every Contact that has a phone, by id.
func storedPhones(ctx context.Context, tx *sql.Tx) ([]storedPhone, error) {
	// raw sql: a Go migration runs on the frozen schema, see phonesToE164.
	rows, err := tx.QueryContext(ctx,
		`SELECT "id", "workspace_id", "phone", "country" FROM "contacts" WHERE "phone" IS NOT NULL ORDER BY "id"`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []storedPhone
	for rows.Next() {
		var s storedPhone
		if err := rows.Scan(&s.id, &s.workspaceID, &s.phone, &s.country); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
