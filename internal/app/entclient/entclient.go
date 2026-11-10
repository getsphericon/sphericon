// Package entclient builds every ent client of the application, with every hook a client carries
// (ADR 0107): the ones a write needs to move another module's log in its own transaction (the
// Workspace and User feeds, the Tracking host log, and the Enterprise Edition's Usage peak). Each
// such hook comes with a guard: a client bound to a transaction runs the hooks, any other client
// runs the guards, which refuse the change the hook would record. It is the wiring's, so the
// database package knows none of those modules, and the core none of the Edition's.
package entclient

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/usertombstones"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspacefeed"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackinghostchanges"
)

// Clients builds the ent clients of one Instance: the core's hooks and guards, then the
// Enterprise Edition's it was made WithEdition. The application builds one (internal/app) and
// the test harness builds its own clients through that one.
type Clients struct {
	hooks, guards []ent.Hook
}

// Core is the clients of the core alone, without the Enterprise Edition's hooks: for a test of a
// core module, and a tool that changes nothing the Edition watches.
var Core Clients

// WithEdition is the clients that carry the Enterprise Edition's hooks with the core's: hooks on
// a client bound to a transaction and guards on any other, as ee.Hooks returns them.
func WithEdition(hooks, guards []ent.Hook) Clients {
	return Clients{hooks: hooks, guards: guards}
}

// New is the plain client: it refuses a change a hook would record (a Workspace or User feed
// change, a Tracking host change, a change of the Usage peak), which is made only inside a
// transaction (events.Bus.WithinTx).
func (c Clients) New(sqlDB *sql.DB) *ent.Client {
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	client.Use(workspacefeed.Guard, usertombstones.Guard, trackinghostchanges.Guard)
	client.Use(c.guards...)
	return client
}

// NewOnTx is a client over a sqlDB whose one connection is already a transaction (go-txdb in
// tests), so a change a hook records takes its place inside that transaction.
func (c Clients) NewOnTx(sqlDB *sql.DB) *ent.Client {
	return c.NewTx(entsql.OpenDB(dialect.Postgres, sqlDB))
}

// NewTx is a client over a driver that runs inside one transaction (the events bus's, which
// events.New is given it for): it carries the hooks where New only guards them. Every client
// bound to a transaction is built here, so the two kinds of client keep the same hooks.
func (c Clients) NewTx(drv dialect.Driver) *ent.Client {
	client := ent.NewClient(ent.Driver(drv))
	client.Use(workspacefeed.Hook, usertombstones.Hook, trackinghostchanges.Hook)
	client.Use(c.hooks...)
	return client
}
