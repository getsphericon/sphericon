package clean

import (
	"context"
	"database/sql"

	entsql "entgo.io/ent/dialect/sql"
)

func statements(ctx context.Context, db *sql.DB) {
	// raw sql: SHOW has no ent form, it is a server command.
	_, _ = db.ExecContext(ctx, "SHOW server_version_num")

	_, _ = db.ExecContext(ctx, "SELECT 1") // raw sql: a one-off probe ent cannot express.

	// raw sql: the reason may run over
	// several lines of comment.
	_, _ = db.ExecContext(ctx,
		"SELECT 1")
}

func fragments() {
	// raw sql: the operator ent lacks.
	_ = entsql.P(func(b *entsql.Builder) { b.WriteString("TRUE") })

	// ent's own predicates are not raw.
	_ = entsql.EQ("a", 1)
}

// runner has a method that merely shares a name with a statement call.
type runner struct{}

func (runner) Exec(ctx context.Context) error { return nil }

func other(ctx context.Context, r runner) { _ = r.Exec(ctx) }
