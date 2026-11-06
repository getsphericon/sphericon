// Package aggregates holds the ent aggregate functions the report modules share.
package aggregates

import (
	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
)

// CountSet counts the rows whose column is set (not NULL), under alias.
func CountSet(column, alias string) ent.AggregateFunc {
	return func(sel *sql.Selector) string {
		return sql.As(sql.Count(sel.C(column)), alias)
	}
}

// CountTrue counts the rows whose boolean column is true, under alias.
func CountTrue(column, alias string) ent.AggregateFunc {
	return func(sel *sql.Selector) string {
		return sql.As("COUNT(*) FILTER (WHERE "+sel.C(column)+")", alias)
	}
}
