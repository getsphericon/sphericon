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
// conflicts with, and a data change waits only for the rows it changes. A VALIDATE CONSTRAINT is
// matched whole, so an ALTER TABLE that adds another action after it is not.
var waitsBlockNoQuery = regexp.MustCompile(`(?is)^(?:` +
	`create\s+(?:unique\s+)?index\s+concurrently\b|` +
	`drop\s+index\s+concurrently\b|` +
	`reindex\b.*\bconcurrently\b|` +
	`(?:update|insert|delete|select|with|set|reset|analyze|vacuum)\b|` +
	`alter\s+table\s+(?:if\s+exists\s+)?(?:only\s+)?\S+\s+validate\s+constraint\s+\S+$)`)

// unboundedWaits is a Blocking finding for each statement of the Up section of a `NO TRANSACTION`
// migration that waitsBlockNoQuery does not match.
func unboundedWaits(file, up string) []Finding {
	var findings []Finding
	for _, st := range statements(up) {
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
	return findings
}

// statement is one statement of a migration and whether a squawk-ignore of UnboundedWaitRule
// stands on a line before it.
type statement struct {
	text    string
	ignored bool
}

// statements splits a goose Up section as Postgres reads it: at every semicolon outside a string
// literal or a quoted identifier, with `--` comments dropped, and a StatementBegin/StatementEnd
// block as one statement. Splitting at line ends instead would let a statement after a trailing
// comment, or a second statement on a line, pass under the first one's head.
func statements(up string) []statement {
	var out []statement
	var text strings.Builder
	ignored, inBlock := false, false
	var quote rune // the quote the scan is inside of, or 0
	flush := func() {
		if t := strings.TrimSpace(text.String()); t != "" {
			out = append(out, statement{text: t, ignored: ignored})
			ignored = false
		}
		text.Reset()
	}
	for line := range strings.Lines(up) {
		trimmed := strings.TrimSpace(line)
		switch {
		case quote == 0 && trimmed == "-- +goose StatementBegin":
			flush()
			inBlock = true
			continue
		case quote == 0 && trimmed == "-- +goose StatementEnd":
			inBlock = false
			flush()
			continue
		case inBlock:
			text.WriteString(line)
			continue
		case quote == 0 && strings.HasPrefix(trimmed, "--"):
			if strings.TrimSpace(text.String()) == "" && ignores(trimmed) {
				ignored = true
			}
			continue
		}
		quote = scanLine(line, quote, &text, flush)
	}
	flush()
	return out
}

// scanLine writes one line into text, calling flush at each semicolon outside quotes and dropping a
// `--` comment, and returns the quote still open at the line's end. A doubled quote inside a
// literal is an escaped one: the scan leaves the literal and enters it again.
func scanLine(line string, quote rune, text *strings.Builder, flush func()) rune {
	runes := []rune(line)
	for i := range runes {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
		case r == '-' && i+1 < len(runes) && runes[i+1] == '-':
			text.WriteByte('\n')
			return quote
		case r == ';':
			flush()
			continue
		}
		text.WriteRune(r)
	}
	return quote
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
