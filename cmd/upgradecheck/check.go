// Command upgradecheck enforces the Upgrade rules of ADR 0033 in CI: a destructive migration needs
// a breaking pull request, and a breaking pull request or commit needs an `Upgrade:` line in its
// `BREAKING CHANGE:` footer that says what a Self-hoster does.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
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

// Finding is one destructive or backward-incompatible diagnostic Atlas reports on a migration.
type Finding struct {
	File string
	Code string
	Text string
}

type atlasFile struct {
	Name    string
	Reports []struct {
		Diagnostics []struct {
			Code string
			Text string
		}
	}
}

// Destructive reads `atlas migrate lint --format '{{ json .Files }}'` and returns the destructive
// (DS) and backward-incompatible (BC) findings of the migrations a person wrote. The generated
// river migrations are skipped.
func Destructive(atlasJSON []byte) ([]Finding, error) {
	var files []atlasFile
	if err := json.Unmarshal(atlasJSON, &files); err != nil {
		return nil, fmt.Errorf("read atlas lint output: %w", err)
	}
	var out []Finding
	for _, f := range files {
		if isRiver(f.Name) {
			continue
		}
		for _, r := range f.Reports {
			for _, d := range r.Diagnostics {
				if strings.HasPrefix(d.Code, "DS") || strings.HasPrefix(d.Code, "BC") {
					out = append(out, Finding{File: f.Name, Code: d.Code, Text: d.Text})
				}
			}
		}
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

// LintWindow is N for `atlas migrate lint --latest N`: how many of the newest migrations (all,
// sorted by name) reach back to the oldest one added that is not a river migration. 0 means
// there is nothing to lint.
func LintWindow(all, added []string) int {
	oldest := -1
	for _, name := range added {
		if isRiver(name) {
			continue
		}
		if i := slices.Index(all, name); i >= 0 && (oldest < 0 || i < oldest) {
			oldest = i
		}
	}
	if oldest < 0 {
		return 0
	}
	return len(all) - oldest
}
