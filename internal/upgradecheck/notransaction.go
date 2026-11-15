package upgradecheck

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"ariga.io/atlas/sql/postgres"
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
// conflicts with, and a data change waits only for the rows it changes. A VALIDATE CONSTRAINT is
// matched whole, so an ALTER TABLE that adds another action after it is not.
var waitsBlockNoQuery = regexp.MustCompile(`(?is)^(?:` +
	`create\s+(?:unique\s+)?index\s+concurrently\b|` +
	`drop\s+index\s+concurrently\b|` +
	`reindex\b.*\bconcurrently\b|` +
	`(?:update|insert|delete|select|with|set|reset|analyze|vacuum)\b|` +
	`alter\s+table\s+(?:if\s+exists\s+)?(?:only\s+)?\S+\s+validate\s+constraint\s+\S+$)`)

// unboundedWaits is a Blocking finding for each statement of the Up section of a `NO TRANSACTION`
// migration that waitsBlockNoQuery does not match. A section it cannot split (a quote or a
// parenthesis left open) is an error: what follows the open quote would pass unread.
func unboundedWaits(file, up string) ([]Finding, error) {
	stmts, err := statements(up)
	if err != nil {
		return nil, fmt.Errorf("%s: split the statements: %w", file, err)
	}
	var findings []Finding
	for _, st := range stmts {
		if st.ignored || waitsBlockNoQuery.MatchString(st.text) {
			continue
		}
		findings = append(findings, Finding{
			File: file,
			Code: UnboundedWaitRule,
			Text: fmt.Sprintf("`%s` waits for its lock without a limit in a NO TRANSACTION migration, and the "+
				"table's queries queue behind the wait: move it to a migration that runs in a transaction, "+
				"where the runner bounds the wait and retries", head(st.text)),
			Class: Blocking,
		})
	}
	return findings, nil
}

// statement is one statement of a migration, without its semicolon, and whether a squawk-ignore of
// UnboundedWaitRule stands in the comments right before it.
type statement struct {
	text    string
	ignored bool
}

// lineComment is a `--` comment inside a statement, which the scanner leaves in its text: the
// allowlist must not read `-- not concurrently` as CONCURRENTLY. A `--` inside a string literal is
// cut too, which only removes text, so a statement can fail the allowlist by it but never pass.
var lineComment = regexp.MustCompile(`--[^\n]*`)

// statements splits a goose Up section as Postgres reads it, with atlas's scanner of the Postgres
// driver: at every semicolon outside a string literal, a quoted identifier, a dollar-quoted body or
// parentheses. Splitting at line ends instead would let a statement after a trailing comment, or a
// second statement on a line, pass under the first one's head. The comments before a statement
// are its own; a blank line ends them. The scanner reads a `--` comment up to its line end, so the
// section gets one: a last comment line would otherwise be read as a statement.
func statements(up string) ([]statement, error) {
	stmts, err := (&postgres.Driver{}).ScanStmts(up + "\n")
	if err != nil {
		return nil, err
	}
	out := make([]statement, 0, len(stmts))
	for _, st := range stmts {
		out = append(out, statement{
			text:    strings.TrimSpace(lineComment.ReplaceAllString(strings.TrimSuffix(st.Text, ";"), "")),
			ignored: slices.ContainsFunc(st.Comments, ignores),
		})
	}
	return out, nil
}

// ignores reports whether a comment is a squawk-ignore naming UnboundedWaitRule.
func ignores(comment string) bool {
	rules, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(comment), "--")), "squawk-ignore")
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
