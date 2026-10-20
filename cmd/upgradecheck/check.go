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
	return loc != nil && upgradeLine.MatchString(message[loc[1]:])
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

// Finding is one destructive diagnostic Squawk reports on a migration.
type Finding struct {
	File string
	Code string
	Text string
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
// Every other Squawk rule is advice (locking, idempotence, naming, integer width) and never fails
// the check. The names come from `squawk --reporter json` run over SQL for each case.
var destructiveRules = map[string]struct{}{
	"ban-drop-column":       {},
	"ban-drop-table":        {},
	"renaming-column":       {},
	"renaming-table":        {},
	"changing-column-type":  {},
	"adding-required-field": {},
}

type squawkFinding struct {
	File     string `json:"file"`
	RuleName string `json:"rule_name"`
	Message  string `json:"message"`
}

// Destructive reads `squawk --reporter json` output and returns the findings whose rule is in
// destructiveRules.
func Destructive(squawkJSON []byte) ([]Finding, error) {
	var found []squawkFinding
	if err := json.Unmarshal(squawkJSON, &found); err != nil {
		return nil, fmt.Errorf("read squawk output: %w", err)
	}
	var out []Finding
	for _, f := range found {
		if _, ok := destructiveRules[f.RuleName]; !ok {
			continue
		}
		out = append(out, Finding{File: filepath.Base(f.File), Code: f.RuleName, Text: f.Message})
	}
	return out, nil
}

// CheckMigrations fails on any finding unless the pull request title is breaking.
func CheckMigrations(title string, findings []Finding) error {
	if len(findings) == 0 || IsBreaking(title) {
		return nil
	}
	var b strings.Builder
	b.WriteString("a destructive migration needs a breaking pull request title (`feat!: ...`) so that release-please cuts a Major:\n")
	for _, f := range findings {
		fmt.Fprintf(&b, "  %s: %s %s\n", f.File, f.Code, f.Text)
	}
	return errors.New(strings.TrimRight(b.String(), "\n"))
}

func isRiver(name string) bool { return strings.Contains(name, "_river_") }
