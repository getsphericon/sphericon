// Package changelog reads the `Upgrade:` entries out of release-please's CHANGELOG.md: what a
// Self-hoster must do for a release (ADR 0034). The binary embeds the changelog of its own
// release (see Embedded), so the notes are readable offline; `sphericon upgrade-notes` and
// `sphericon upgrade` both read them through this package.
package changelog

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// Release is one `## ` section of the changelog.
type Release struct {
	// Version is the release number without a leading "v" ("0.2.0").
	Version string
	// Upgrades are the release's `Upgrade:` entries, in file order, with the marker removed.
	Upgrades []string
}

var (
	// "## [0.2.0](compare-url) (date)" or "## 0.1.0 (date)".
	heading = regexp.MustCompile(`^##\s+\[?v?(\d+\.\d+\.\d+[0-9A-Za-z.+-]*?)\]?(?:[\s(]|$)`)
	// An entry may sit in a bullet, behind a bold scope ("**api:** ") and a bold marker.
	upgradeLine = regexp.MustCompile(`^\s*(?:[-*]\s+)?(?:\*\*[^*]*:\*\*\s+)*(?:\*\*)?Upgrade:(?:\*\*)?\s*(.*)$`)
)

// Parse returns the releases of a changelog in file order (newest first).
func Parse(text string) []Release {
	var releases []Release
	open := false // the last Upgrade line may continue on the next indented line
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := heading.FindStringSubmatch(line); m != nil {
			releases = append(releases, Release{Version: m[1]})
			open = false
			continue
		}
		if len(releases) == 0 {
			continue
		}
		cur := &releases[len(releases)-1]
		if m := upgradeLine.FindStringSubmatch(line); m != nil {
			cur.Upgrades = append(cur.Upgrades, strings.TrimSpace(m[1]))
			open = true
			continue
		}
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		trimmed := strings.TrimSpace(line)
		if open && indented && trimmed != "" && !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") {
			last := len(cur.Upgrades) - 1
			cur.Upgrades[last] += " " + trimmed
			continue
		}
		open = false
	}
	return releases
}

// Since keeps the releases newer than since ("v0.1.9" or "0.1.9") that have Upgrade entries.
func Since(releases []Release, since string) ([]Release, error) {
	base := "v" + strings.TrimPrefix(since, "v")
	if !semver.IsValid(base) {
		return nil, fmt.Errorf("%q is not a version", since)
	}
	var out []Release
	for _, r := range releases {
		if len(r.Upgrades) > 0 && semver.Compare("v"+r.Version, base) > 0 {
			out = append(out, r)
		}
	}
	return out, nil
}

// Render prints releases as a version line followed by one bullet per entry, a blank line
// between releases; nothing for no releases.
func Render(releases []Release) string {
	var b strings.Builder
	for i, r := range releases {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(r.Version + "\n")
		for _, u := range r.Upgrades {
			b.WriteString("  - " + u + "\n")
		}
	}
	return b.String()
}
