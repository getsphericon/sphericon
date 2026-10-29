package upgradecheck

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// collapseLine marks a pull request that rewrites migration history on purpose (a squash of the
// migrations into a new baseline): a body line `History-Collapse: <reason>`. Anything else that
// touches a migration already on main is refused.
var collapseLine = regexp.MustCompile(`(?m)^History-Collapse:[ \t]*\S`)

// IsHistoryCollapse reports whether a pull request body carries the History-Collapse marker.
func IsHistoryCollapse(body string) bool { return collapseLine.MatchString(body) }

// isMigrationFile reports whether a path under migrations/ is a real migration (a goose .sql file
// or a Go migration), not a bookkeeping file.
func isMigrationFile(p string) bool {
	if path.Dir(p) != "migrations" {
		return false
	}
	ext := path.Ext(p)
	return ext == ".sql" || ext == ".go"
}

// Immutability returns the migration files a pull request changed in place or removed, sorted
// and without duplicates: an applied migration is never rewritten. A history collapse reports
// none; added files never appear because only modified and deleted paths are passed in.
func Immutability(modified, deleted []string, collapse bool) []string {
	if collapse {
		return nil
	}
	var out []string
	for _, p := range slices.Concat(modified, deleted) {
		if isMigrationFile(p) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// CheckImmutability fails naming each violating file.
func CheckImmutability(modified, deleted []string, collapse bool) error {
	bad := Immutability(modified, deleted, collapse)
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("a migration already on main is never edited or deleted (ADR 0033); changed: %s. "+
		"Add a new migration instead, or mark a deliberate history collapse with a `History-Collapse: <reason>` "+
		"line in the pull request body", strings.Join(bad, ", "))
}
