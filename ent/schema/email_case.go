package schema

import "entgo.io/ent/dialect/entsql"

// LowerCheck is the CHECK constraint that keeps an address column lower-cased, named
// <table>_<column>_lower: every stored address is in its Email case (GLOSSARY), so one address is
// one row whatever case it was typed in, and the unique indexes on it hold case-insensitively. The
// owner of the column puts each address it writes in its Email case (internal/kernel/emailcase);
// this makes the database refuse a write that skipped it, so a missed path fails loudly instead of
// making a second account or Contact (ADR 0107). A Destination of another Channel (a phone number
// in E.164, a Telegram user id, the hash an Erasure leaves) has no upper case, so it passes.
func LowerCheck(table, column string) (name, expr string) {
	return table + "_" + column + "_lower", column + " = lower(" + column + ")"
}

// WithLowerCheck is a with the LowerCheck of each column added to its CHECK constraints.
func WithLowerCheck(a entsql.Annotation, columns ...string) entsql.Annotation {
	if a.Checks == nil {
		a.Checks = map[string]string{}
	}
	for _, c := range columns {
		name, expr := LowerCheck(a.Table, c)
		a.Checks[name] = expr
	}
	return a
}
