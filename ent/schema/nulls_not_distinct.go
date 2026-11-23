package schema

// nullsNotDistinct are the unique indexes, by name, whose NULLs count as one value (NULLS NOT
// DISTINCT, Postgres 15 and later, postgres.MinMajor): a row whose column is null conflicts with
// another whose column is null too. ent's index annotation cannot say it, so the migration's diff
// (internal/infra/devdb) adds it to these indexes of the ent schema.
var nullsNotDistinct = map[string]bool{
	ActiveEnrollmentIndex: true,
}

// NullsNotDistinct is whether the unique index of the name counts its NULLs as one value.
func NullsNotDistinct(index string) bool {
	return nullsNotDistinct[index]
}
