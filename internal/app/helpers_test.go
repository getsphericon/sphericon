package app

import (
	"database/sql"

	"github.com/samber/do/v2"

	"github.com/getsphericon/sphericon/ent"
)

// invokeEnt resolves the app's ent client from its container, the same instance the
// operator methods use.
func invokeEnt(a *App) (*ent.Client, error) {
	c, err := do.Invoke[*entClient](a.injector)
	if err != nil {
		return nil, err
	}
	return c.Client, nil
}

// invokeSQL resolves the app's database pool, the one its outbox writes through.
func invokeSQL(a *App) (*sql.DB, error) {
	d, err := do.Invoke[*sqlDB](a.injector)
	if err != nil {
		return nil, err
	}
	return d.DB, nil
}
