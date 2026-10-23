package schema

// LowerEmailCheck is the CHECK constraint that keeps table's email column lower-cased, named
// <table>_email_lower. An account address is stored lower-cased, so one address is one row
// whatever case it was typed in and the unique index on the column holds case-insensitively;
// internal/emailcase lower-cases it on every write through ent, and this makes the database
// refuse a write that went around it, so a missed path fails loudly instead of making a second
// account.
func LowerEmailCheck(table string) (name, expr string) {
	return table + "_email_lower", "email = lower(email)"
}
