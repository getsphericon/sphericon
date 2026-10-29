// Package prcheck checks what a pull request body must say beyond its code: how an AI tool was
// used (the "AI assistance" section of the pull request template) and why each `contract-copy:`
// marker the change adds is justified (a `Contract-Copy:` line per file). The body is the
// contributor's own text, so `cmd/prcheck` runs the check when the body is edited, not only when
// code is pushed.
package prcheck

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const disclosureHeading = "## AI assistance"

var (
	checkedBox      = regexp.MustCompile(`^\s*[-*]\s*\[[xX]\]\s*\S`)
	contractCopy    = regexp.MustCompile(`(//|#)\s*contract-copy:\s*\S`)
	declarationLine = regexp.MustCompile(`(?m)^Contract-Copy:[ \t]*(\S+?):[ \t]*\S`)
)

// A marker is a comment in code or in a fixture. Docs and the two places that hold marker text on
// purpose (the checks that read markers) are text about a marker, not one.
var (
	markerSuffixes = []string{".go", ".ts", ".tsx", ".yml"}
	markerExempt   = []string{"internal/contractcopy/", "internal/prcheck/"}
)

// CheckDisclosure fails unless the body has an "AI assistance" section with exactly one checked
// option: the person who opens the pull request says how an AI tool was used, in one place a
// reviewer reads first.
func CheckDisclosure(body string) error {
	text, ok := sectionOf(body, disclosureHeading)
	if !ok {
		return fmt.Errorf("the pull request body has no %q section: keep the section of the pull request template "+
			"and check the one option that says how an AI tool was used", disclosureHeading)
	}
	checked := 0
	for line := range strings.Lines(text) {
		if checkedBox.MatchString(line) {
			checked++
		}
	}
	if checked != 1 {
		return fmt.Errorf("check exactly one option under %q (found %d checked): the one that says how an AI tool was used",
			disclosureHeading, checked)
	}
	return nil
}

// sectionOf returns the text under a `## ` heading, up to the next `## ` heading, and whether the
// heading is there.
func sectionOf(body, heading string) (string, bool) {
	var out []string
	found := false
	for line := range strings.Lines(body) {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			if found {
				break
			}
			found = trimmed == heading
			continue
		}
		if found {
			out = append(out, line)
		}
	}
	return strings.Join(out, ""), found
}

// AddedMarkerFiles returns the files, sorted and without repeats, in which a unified diff adds a
// line with a `contract-copy: <reason>` marker.
func AddedMarkerFiles(diff string) []string {
	var files []string
	var current string
	for line := range strings.Lines(diff) {
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "+++ "):
			current = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
		case strings.HasPrefix(line, "+") && contractCopy.MatchString(line) && carriesMarkers(current):
			files = append(files, current)
		}
	}
	slices.Sort(files)
	return slices.Compact(files)
}

func carriesMarkers(path string) bool {
	return slices.ContainsFunc(markerSuffixes, func(s string) bool { return strings.HasSuffix(path, s) }) &&
		!slices.ContainsFunc(markerExempt, func(p string) bool { return strings.HasPrefix(path, p) })
}

// CheckMarkers fails for each file that adds a `contract-copy:` marker the body does not declare
// with a `Contract-Copy: <file>: <reason>` line. A marker exempts a value from rule 6; the
// declaration puts that exemption in front of the reviewer instead of in a diff nobody reads.
func CheckMarkers(files []string, body string) error {
	declared := map[string]bool{}
	for _, m := range declarationLine.FindAllStringSubmatch(body, -1) {
		declared[m[1]] = true
	}
	var missing []string
	for _, f := range files {
		if !declared[f] {
			missing = append(missing, "Contract-Copy: "+f+": <why this value is not a copy of the contract>")
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return errors.New("the change adds `contract-copy:` markers the pull request body does not declare; add a line " +
		"for each to the body:\n  " + strings.Join(missing, "\n  "))
}
