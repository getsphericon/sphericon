// Package prcheck checks what a pull request body must say beyond its code: how an AI tool was
// used (the "AI assistance" section of the pull request template). The body is the contributor's
// own text, so `cmd/prcheck` runs the check when the body is edited, not only when code is pushed.
package prcheck

import (
	"fmt"
	"regexp"
	"strings"
)

const disclosureHeading = "## AI assistance"

var checkedBox = regexp.MustCompile(`^\s*[-*]\s*\[[xX]\]\s*\S`)

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
