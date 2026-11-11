package schema

import (
	"strings"

	"entgo.io/ent/dialect/entsql"
)

// FormatCheck is the CHECK that keeps a column in the form its owner normalises it to, named
// <table>_<column>_format: pattern is the normaliser's own (a phone's E.164, a domain's punycode),
// so the owner and the database read the same rule, and a write that skipped the normaliser fails
// loudly instead of storing a second spelling of one key (ADR 0107). A NULL passes.
func FormatCheck(table, column, pattern string) (name, expr string) {
	return table + "_" + column + "_format", column + " ~ '" + strings.ReplaceAll(pattern, "'", "''") + "'"
}

// WithFormatCheck is a with the FormatCheck of column added to its CHECK constraints.
func WithFormatCheck(a entsql.Annotation, column, pattern string) entsql.Annotation {
	if a.Checks == nil {
		a.Checks = map[string]string{}
	}
	name, expr := FormatCheck(a.Table, column, pattern)
	a.Checks[name] = expr
	return a
}

// currencyPattern is an ISO 4217 code in capitals, the form internal/kernel/money reads.
const currencyPattern = `^[A-Z]{3}$`

// CurrencyCheck is the FormatCheck of a currency column.
func CurrencyCheck(table, column string) (name, expr string) {
	return FormatCheck(table, column, currencyPattern)
}
