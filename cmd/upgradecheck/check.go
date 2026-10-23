// Command upgradecheck enforces the Upgrade rules of ADR 0033 in CI: a destructive migration needs
// a breaking pull request, and a breaking pull request or commit needs an `Upgrade:` line in its
// `BREAKING CHANGE:` footer that says what a Self-hoster does.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var (
	breakingSubject = regexp.MustCompile(`^[a-zA-Z]+(\([^)]*\))?!: `)
	breakingFooter  = regexp.MustCompile(`(?m)^BREAKING[ -]CHANGE: `)
	upgradeLine     = regexp.MustCompile(`(?m)^Upgrade:[ \t]*\S`)
)

// IsBreaking reports whether a commit message (or a pull request title) is a breaking one:
// a `!` before the colon of its subject, or a `BREAKING CHANGE:` footer.
func IsBreaking(message string) bool {
	subject, _, _ := strings.Cut(message, "\n")
	return breakingSubject.MatchString(subject) || breakingFooter.MatchString(message)
}

// hasUpgrade reports whether a message carries an `Upgrade:` line inside its `BREAKING CHANGE:`
// footer text: release-please renders only that text into CHANGELOG.md, never a footer of its own.
// The line may follow the marker directly or start a later line of the footer.
func hasUpgrade(message string) bool {
	loc := breakingFooter.FindStringIndex(message)
	return len(loc) == 2 && upgradeLine.MatchString(message[loc[1]:])
}

// CheckBreaking fails when the title or a commit is breaking and neither a commit message nor
// the pull request body has an `Upgrade:` line in its `BREAKING CHANGE:` footer.
func CheckBreaking(title, body string, commits []string) error {
	breaking := IsBreaking(title) || slices.ContainsFunc(commits, IsBreaking)
	if !breaking {
		return nil
	}
	if hasUpgrade(body) || slices.ContainsFunc(commits, hasUpgrade) {
		return nil
	}
	return errors.New("a breaking change needs an `Upgrade:` line inside its `BREAKING CHANGE:` footer, in a commit message " +
		"or the pull request body, that says what a Self-hoster does (release-please renders only that footer into " +
		"CHANGELOG.md), for example `BREAKING CHANGE: the env var is renamed\nUpgrade: set SPHERICON_X before starting`")
}

// Class says what a Squawk finding costs a pull request.
type Class int

const (
	// Destructive findings strand data or break an older release: they need a breaking title.
	Destructive Class = iota
	// Blocking findings hold a lock on a live table: they fail under any title.
	Blocking
)

// Finding is one diagnostic Squawk reports on a migration that fails the check.
type Finding struct {
	File  string
	Code  string
	Text  string
	Class Class
}

// destructiveRules is the set of Squawk rules that count as destructive for ADR 0033:
//
//	ban-drop-column       a dropped column
//	ban-drop-table        a dropped table
//	renaming-column       a renamed column
//	renaming-table        a renamed table
//	changing-column-type  a changed column type
//	adding-required-field a NOT NULL column added without a default
//
// The names come from `squawk --reporter json` run over SQL for each case.
var destructiveRules = map[string]struct{}{
	"ban-drop-column":       {},
	"ban-drop-table":        {},
	"renaming-column":       {},
	"renaming-table":        {},
	"changing-column-type":  {},
	"adding-required-field": {},
}

// blockingRules is the set of Squawk rules that count as blocking for ADR 0033: a statement that
// holds a lock long enough to stop writes on a table a running Instance uses.
//
//	require-concurrent-index-creation  CREATE INDEX without CONCURRENTLY
//	require-concurrent-index-deletion  DROP INDEX without CONCURRENTLY
//	require-concurrent-reindex         REINDEX without CONCURRENTLY
//	disallowed-unique-constraint       ADD CONSTRAINT UNIQUE (build a unique index first)
//	adding-foreign-key-constraint      ADD CONSTRAINT FOREIGN KEY without NOT VALID
//	constraint-missing-not-valid       ADD CONSTRAINT (CHECK, FOREIGN KEY) without NOT VALID
//	adding-not-nullable-field          SET NOT NULL (a full scan under ACCESS EXCLUSIVE)
//	adding-field-with-default          ADD COLUMN with a volatile default (a table rewrite)
//	adding-serial-primary-key-field    ADD COLUMN serial PRIMARY KEY (a table rewrite)
//
// A statement the author has checked is allowed with `-- squawk-ignore <rule>` on the line before
// it, which a reviewer sees in the diff. Every other Squawk rule is advice (idempotence, naming,
// integer width, lock_timeout) and never fails the check.
var blockingRules = map[string]struct{}{
	"require-concurrent-index-creation": {},
	"require-concurrent-index-deletion": {},
	"require-concurrent-reindex":        {},
	"disallowed-unique-constraint":      {},
	"adding-foreign-key-constraint":     {},
	"constraint-missing-not-valid":      {},
	"adding-not-nullable-field":         {},
	"adding-field-with-default":         {},
	"adding-serial-primary-key-field":   {},
}

type squawkFinding struct {
	File     string `json:"file"`
	RuleName string `json:"rule_name"`
	Message  string `json:"message"`
}

// Findings reads `squawk --reporter json` output and returns the findings whose rule is in
// destructiveRules or blockingRules.
func Findings(squawkJSON []byte) ([]Finding, error) {
	var found []squawkFinding
	if err := json.Unmarshal(squawkJSON, &found); err != nil {
		return nil, fmt.Errorf("read squawk output: %w", err)
	}
	var out []Finding
	for _, f := range found {
		finding := Finding{File: filepath.Base(f.File), Code: f.RuleName, Text: f.Message}
		if _, ok := destructiveRules[f.RuleName]; ok {
			finding.Class = Destructive
			out = append(out, finding)
		}
		if _, ok := blockingRules[f.RuleName]; ok {
			finding.Class = Blocking
			out = append(out, finding)
		}
	}
	return out, nil
}

// CheckMigrations fails on every blocking finding, and on every destructive one unless the pull
// request title is breaking.
func CheckMigrations(title string, findings []Finding) error {
	var blocking, destructive []Finding
	for _, f := range findings {
		switch f.Class {
		case Blocking:
			blocking = append(blocking, f)
		case Destructive:
			destructive = append(destructive, f)
		}
	}
	if IsBreaking(title) {
		destructive = nil
	}
	var b strings.Builder
	if len(blocking) > 0 {
		b.WriteString("a migration must not lock a live table: build indexes `CONCURRENTLY` under `-- +goose NO TRANSACTION`, " +
			"add constraints `NOT VALID` and validate them in a later statement, or put `-- squawk-ignore <rule>` " +
			"on the line before a statement you have checked (a table that is empty or created in this migration needs none):\n")
		writeFindings(&b, blocking)
	}
	if len(destructive) > 0 {
		b.WriteString("a destructive migration needs a breaking pull request title (`feat!: ...`) so that release-please cuts a Major:\n")
		writeFindings(&b, destructive)
	}
	if b.Len() == 0 {
		return nil
	}
	return errors.New(strings.TrimRight(b.String(), "\n"))
}

func writeFindings(b *strings.Builder, findings []Finding) {
	for _, f := range findings {
		fmt.Fprintf(b, "  %s: %s %s\n", f.File, f.Code, f.Text)
	}
}

func isRiver(name string) bool { return strings.Contains(name, "_river_") }
