package bad

import (
	"context"
	"database/sql"

	entsql "entgo.io/ent/dialect/sql"
)

func statements(ctx context.Context, db *sql.DB, tx *sql.Tx) {
	_, _ = db.ExecContext(ctx, "SELECT 1")  // want `^rule 4: raw SQL statement .*; fix: build it with ent .*raw sql[:] <reason>`
	_ = db.QueryRowContext(ctx, "SELECT 1") // want `^rule 4: raw SQL statement`
	_, _ = tx.QueryContext(ctx, "SELECT 1") // want `^rule 4: raw SQL statement`
	_, _ = db.Exec("SELECT 1")              // want `^rule 4: raw SQL statement`

	// raw sql:
	_, _ = db.ExecContext(ctx, "SELECT 1") // want `^rule 4: raw SQL statement`

	// An ordinary comment is not a reason.
	_, _ = db.ExecContext(ctx, "SELECT 1") // want `^rule 4: raw SQL statement`
}

func fragments() {
	_ = entsql.P(func(b *entsql.Builder) { b.WriteString("TRUE") }) // want `^rule 4: raw SQL fragment`
	_ = entsql.ExprFunc(func(b *entsql.Builder) {})                 // want `^rule 4: raw SQL fragment`
	_ = entsql.Expr("1")                                            // want `^rule 4: raw SQL fragment`
	_ = entsql.Raw("1")                                             // want `^rule 4: raw SQL fragment`
}
