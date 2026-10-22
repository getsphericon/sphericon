// Package sql is a stub of entgo.io/ent/dialect/sql for the analyzer's test data.
package sql

type Builder struct{}

func (b *Builder) WriteString(string) *Builder { return b }

type Predicate struct{}

type Querier interface{}

func P(func(*Builder)) *Predicate     { return nil }
func ExprFunc(func(*Builder)) Querier { return nil }
func Expr(string, ...any) Querier     { return nil }
func Raw(string) Querier              { return nil }
func EQ(string, any) *Predicate       { return nil }
