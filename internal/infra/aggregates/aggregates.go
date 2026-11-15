// Package aggregates holds the ent aggregate functions the report modules share.
package aggregates

import (
	"strings"

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
		return countWhere(sel.C(column), alias)
	}
}

// CountEQ counts the rows whose column holds value, under alias. value is a constant of the code
// (an enum's value, an event's name), written into the statement as a quoted literal.
func CountEQ(column, value, alias string) ent.AggregateFunc {
	return func(sel *sql.Selector) string {
		return countWhere(sel.C(column)+" = '"+strings.ReplaceAll(value, "'", "''")+"'", alias)
	}
}

func countWhere(condition, alias string) string {
	// raw sql: ent has no filtered aggregate (COUNT(*) FILTER (WHERE ...)), and an aggregate is
	// text, so its condition cannot bind an argument.
	return sql.As("COUNT(*) FILTER (WHERE "+condition+")", alias)
}
