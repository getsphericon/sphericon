package db

import (
	"database/sql"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/getsphericon/sphericon/ent"
)

func NewEntClient(sqlDB *sql.DB) *ent.Client {
	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	return ent.NewClient(ent.Driver(drv))
}
