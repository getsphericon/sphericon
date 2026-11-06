// Package entclient builds the application's ent clients with the hooks every client carries: the
// lower-casing of stored addresses and the change positions of the Workspace and User feeds and of
// the Tracking host log. It is the wiring's, so the database package knows none of those modules.
package entclient

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/contacts/emailcase"
	"github.com/getsphericon/sphericon/internal/domain/identity/usertombstones"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspacefeed"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackinghostchanges"
)

// New is the plain client: it refuses a change to the Workspace and User feeds, which
// is made only inside a transaction (events.Bus.WithinTx).
func New(sqlDB *sql.DB) *ent.Client {
	c := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	c.Use(emailcase.Hook, workspacefeed.Guard, usertombstones.Guard, trackinghostchanges.Guard)
	return c
}

// NewOnTx is a client over a sqlDB whose one connection is already a
// transaction (go-txdb in tests), so a change to the Workspace or User feed takes its position
// inside that transaction.
func NewOnTx(sqlDB *sql.DB) *ent.Client {
	return NewTx(entsql.OpenDB(dialect.Postgres, sqlDB))
}

// NewTx is a client over a driver that runs inside one transaction (the events bus's, which
// events.New is given it for): it carries the hooks that write the feeds where New only guards
// them. Every client bound to a transaction is built here, so the two kinds of client keep the
// same hooks.
func NewTx(drv dialect.Driver) *ent.Client {
	c := ent.NewClient(ent.Driver(drv))
	c.Use(emailcase.Hook, workspacefeed.Hook, usertombstones.Hook, trackinghostchanges.Hook)
	return c
}
