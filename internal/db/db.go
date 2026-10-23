package db

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/emailcase"
	"github.com/getsphericon/sphericon/internal/trackinghostlog"
	"github.com/getsphericon/sphericon/internal/workspacefeed"
)

// NewEntClient is the plain client: it refuses a change to the Workspace feed, which
// is made only inside a transaction (events.Bus.WithinTx).
func NewEntClient(sqlDB *sql.DB) *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	c := ent.NewClient(ent.Driver(drv))
	c.Use(emailcase.Hook, workspacefeed.Guard, trackinghostlog.Guard)
	return c
}

// NewEntClientOnTx is a client over a sqlDB whose one connection is already a
// transaction (go-txdb in tests), so a change to the Workspace feed takes its position
// inside that transaction.
func NewEntClientOnTx(sqlDB *sql.DB) *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	c := ent.NewClient(ent.Driver(drv))
	c.Use(emailcase.Hook, workspacefeed.Hook, trackinghostlog.Hook)
	return c
}
