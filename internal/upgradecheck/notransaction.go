package upgradecheck

import (
	"fmt"
	"regexp"
	"strings"
)

// UnboundedWaitRule is the code of a statement that waits for a lock queries queue behind, in a
// migration without a transaction. The runner bounds the lock wait of a migration that runs in a
// transaction and retries it, since Postgres rolls it back whole; a `NO TRANSACTION` migration
// keeps an unbounded wait, so it may hold only statements whose wait blocks no query (ADR 0033).
// A statement the author has checked, on a table no query reads yet, carries
// `-- squawk-ignore unbounded-lock-wait` on the line before it, as a Squawk rule's would.
const UnboundedWaitRule = "unbounded-lock-wait"

// waitsBlockNoQuery matches the statements a `NO TRANSACTION` migration may hold: an index built,
// dropped or rebuilt CONCURRENTLY and a constraint validated take a lock that no read or write
// conflicts with, and a data change waits only for the rows it changes.
var waitsBlockNoQuery = regexp.MustCompile(`(?is)^(` +
	`create\s+(unique\s+)?index\s+concurrently\b|` +
	`drop\s+index\s+concurrently\b|` +
	`reindex\b.*\bconcurrently\b|` +
	`alter\s+table\s+(if\s+exists\s+)?(only\s+)?\S+\s+validate\s+constraint\b|` +
	`(update|insert|delete|select|with|set|reset|analyze|vacuum)\b)`)

// unboundedWaits is a Blocking finding for each statement of the Up section of a `NO TRANSACTION`
// migration that waitsBlockNoQuery does not match. Statements are split as goose splits them: at
// a line that ends with a semicolon, or around a StatementBegin/StatementEnd block.
func unboundedWaits(file, up string) []Finding {
	var findings []Finding
	var stmt strings.Builder
	ignored, inBlock := false, false
	flush := func() {
		text := strings.TrimSpace(stmt.String())
		stmt.Reset()
		if text != "" && !ignored && !waitsBlockNoQuery.MatchString(text) {
			findings = append(findings, Finding{
				File: file,
				Code: UnboundedWaitRule,
				Text: fmt.Sprintf("`%s` waits for its lock without a limit in a NO TRANSACTION migration, and the "+
					"table's queries queue behind the wait: move it to a migration that runs in a transaction, "+
					"where the runner bounds the wait and retries", head(text)),
				Class: Blocking,
			})
		}
		ignored = false
	}
	for line := range strings.Lines(up) {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "-- +goose StatementBegin":
			inBlock = true
			continue
		case trimmed == "-- +goose StatementEnd":
			inBlock = false
			flush()
			continue
		case !inBlock && strings.HasPrefix(trimmed, "--"):
			if stmt.Len() == 0 && ignores(trimmed) {
				ignored = true
			}
			continue
		case trimmed == "":
			continue
		}
		stmt.WriteString(line)
		if !inBlock && strings.HasSuffix(trimmed, ";") {
			flush()
		}
	}
	flush()
	return findings
}

// ignores reports whether a comment line is a squawk-ignore naming UnboundedWaitRule.
func ignores(comment string) bool {
	rules, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(comment, "--")), "squawk-ignore")
	if !ok {
		return false
	}
	for rule := range strings.SplitSeq(rules, ",") {
		if strings.TrimSpace(rule) == UnboundedWaitRule {
			return true
		}
	}
	return false
}

// head is the start of a statement, on one line, for a finding to name it by.
func head(stmt string) string {
	const width = 60
	text := strings.Join(strings.Fields(stmt), " ")
	if len(text) > width {
		return text[:width] + "…"
	}
	return text
}
