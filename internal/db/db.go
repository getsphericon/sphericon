package db

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/workspacefeed"
)

func NewEntClient(sqlDB *sql.DB) *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	c := ent.NewClient(ent.Driver(drv))
	c.Use(workspacefeed.Hook)
	return c
}
