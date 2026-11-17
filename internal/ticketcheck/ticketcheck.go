// Package ticketcheck checks that a ticket ready for an agent builds on decisions that are built. An
// ADR says in its front matter whether it is built (`build:`). A ticket that names an ADR not built
// yet, or whose own ADR names one, either waits for a ticket that builds it or says why it builds
// without it: an agent handed the ticket otherwise finds nothing to build on. `cmd/ticketcheck` runs
// the checks on the files of docs/adr and on the issues of the tracker.
package ticketcheck

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// RecordedFrom is the first ADR number that must carry a `build:` field. An earlier ADR without one
// counts as built: the field was introduced after them, and the ones known not to be built carry it.
const RecordedFrom = 178

// The values of the `build:` field besides a list of ticket numbers.
const (
	buildNone = "none" // not built, and no ticket builds it yet
	buildDone = "done" // built, or decides nothing to build
)

// ADR is one decision record of docs/adr.
type ADR struct {
	Number string // the four digits of its file name
	Text   string
	Build  Build
}

// Build is what an ADR's `build:` field says.
type Build struct {
	Recorded bool  // the field is there
	Done     bool  // `done`: built, or nothing to build
	Tickets  []int // the tickets that build it; empty with `none`
}

// ParseADR reads an ADR file's number from its name and its `build:` field from its front matter.
func ParseADR(name, text string) (ADR, error) {
	base := filepath.Base(name)
	if len(base) < 5 || base[4] != '-' || strings.Trim(base[:4], "0123456789") != "" {
		return ADR{}, fmt.Errorf("%s: an ADR file is named <four digits>-<title>.md", name)
	}
	build, err := parseBuild(text)
	if err != nil {
		return ADR{}, fmt.Errorf("%s: %w", name, err)
	}
	return ADR{Number: base[:4], Text: text, Build: build}, nil
}

var errBuildValue = errors.New("`build:` is `none` (not built, no ticket yet), `done` (built, or nothing to build) " +
	"or the list of the tickets that build it, such as `[1456, 1457]`")

// parseBuild reads the `build:` line of the front matter. The line is read on its own, not as YAML:
// an older ADR's `status:` line is free text that YAML does not parse.
func parseBuild(text string) (Build, error) {
	front, ok := frontMatter(text)
	if !ok {
		return Build{}, nil
	}
	for line := range strings.Lines(front) {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "build:")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch value {
		case buildNone:
			return Build{Recorded: true}, nil
		case buildDone:
			return Build{Recorded: true, Done: true}, nil
		}
		list, ok := strings.CutPrefix(value, "[")
		list, closed := strings.CutSuffix(list, "]")
		if !ok || !closed || strings.TrimSpace(list) == "" {
			return Build{}, errBuildValue
		}
		var tickets []int
		for item := range strings.SplitSeq(list, ",") {
			number, err := strconv.Atoi(strings.TrimSpace(item))
			if err != nil || number <= 0 {
				return Build{}, errBuildValue
			}
			tickets = append(tickets, number)
		}
		return Build{Recorded: true, Tickets: tickets}, nil
	}
	return Build{}, nil
}

// frontMatter returns the YAML between the opening `---` line and the next one.
func frontMatter(text string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.ReplaceAll(text, "\r\n", "\n"), "---\n")
	if !ok {
		return "", false
	}
	front, _, ok := strings.Cut(rest, "\n---")
	return front, ok
}

// CheckADRs fails on an ADR from RecordedFrom on that does not say whether it is built.
func CheckADRs(adrs []ADR) error {
	var errs []error
	for _, adr := range adrs {
		if adr.Number >= fmt.Sprintf("%04d", RecordedFrom) && !adr.Build.Recorded {
			errs = append(errs, fmt.Errorf("ADR %s has no `build:` in its front matter: %w", adr.Number, errBuildValue))
		}
	}
	return errors.Join(errs...)
}

// Issue is a ticket of the tracker.
type Issue struct {
	Number    int
	Title     string
	Body      string
	BlockedBy []int // the tracker's native blocking edges
}

// Gap is an ADR a ticket builds on that is not built, with the open tickets that build it.
type Gap struct {
	ADR     string
	Tickets []int
}

func (g Gap) String() string {
	if len(g.Tickets) == 0 {
		return fmt.Sprintf("ADR %s is not built, and no ticket builds it yet", g.ADR)
	}
	refs := make([]string, len(g.Tickets))
	for i, n := range g.Tickets {
		refs[i] = "#" + strconv.Itoa(n)
	}
	return fmt.Sprintf("ADR %s is not built yet: %s builds it", g.ADR, strings.Join(refs, ", "))
}

var (
	// "ADR 0136", "ADRs 0122, 0124 and 0125", "ADR-0007".
	adrList         = regexp.MustCompile(`\bADRs?[\s-]+(\d{4}(?:(?:\s*,\s*|\s*,?\s+and\s+)\d{4})*)`)
	adrLink         = regexp.MustCompile(`(?:\[\[|docs/adr/)(\d{4})-`)
	fourDigit       = regexp.MustCompile(`\d{4}`)
	ownADR          = regexp.MustCompile(`\(ADR (\d{4})\)`)
	issueRef        = regexp.MustCompile(`#(\d+)`)
	blockedBy       = regexp.MustCompile(`(?i)^\W*blocked by\b(.*)$`)
	markdownHeading = regexp.MustCompile(`^\s*#{1,6}\s`)
)

// References returns the ADR numbers a text names, sorted.
func References(text string) []string {
	lists, links := adrList.FindAllStringSubmatch(text, -1), adrLink.FindAllStringSubmatch(text, -1)
	refs := make([]string, 0, len(lists)+len(links))
	for _, m := range lists {
		refs = append(refs, fourDigit.FindAllString(m[1], -1)...)
	}
	for _, m := range links {
		refs = append(refs, m[1])
	}
	slices.Sort(refs)
	return slices.Compact(refs)
}

// Check returns the ADRs the issue builds on that are not built and that it neither waits for nor
// waives. The issue builds on every ADR its title and body name and every ADR its own ADRs name;
// its own ADRs are the one its title names as `(ADR NNNN)` and those whose `build:` lists it. An ADR
// is built when it has no `build:` field, has `done`, or lists only tickets that are not open. The
// issue waits for an ADR when it is blocked by one of that ADR's open tickets (a native edge or a
// `Blocked by` line), and waives it with a line `Without ADR NNNN: <why>`.
func Check(issue Issue, adrs []ADR, open func(int) bool) []Gap {
	byNumber := make(map[string]ADR, len(adrs))
	own := map[string]bool{}
	for _, adr := range adrs {
		byNumber[adr.Number] = adr
		if slices.Contains(adr.Build.Tickets, issue.Number) {
			own[adr.Number] = true
		}
	}
	for _, m := range ownADR.FindAllStringSubmatch(issue.Title, -1) {
		own[m[1]] = true
	}

	refs := References(issue.Title + "\n" + issue.Body)
	for number := range own {
		// An ADR builds on earlier ones; a later one it names amends it ("Amended by").
		for _, ref := range References(byNumber[number].Text) {
			if ref < number {
				refs = append(refs, ref)
			}
		}
	}
	slices.Sort(refs)
	refs = slices.Compact(refs)

	waits := append(slices.Clone(issue.BlockedBy), blockers(issue.Body)...)

	var gaps []Gap
	for _, number := range refs {
		adr, ok := byNumber[number]
		if own[number] || !ok || !adr.Build.Recorded || adr.Build.Done {
			continue
		}
		var building []int
		for _, n := range adr.Build.Tickets {
			if open(n) {
				building = append(building, n)
			}
		}
		if len(adr.Build.Tickets) > 0 && len(building) == 0 {
			continue
		}
		if slices.ContainsFunc(building, func(n int) bool { return slices.Contains(waits, n) }) || waives(issue.Body, number) {
			continue
		}
		gaps = append(gaps, Gap{ADR: number, Tickets: building})
	}
	return gaps
}

// blockers returns the tickets a body says it waits for: those on a `Blocked by` line, and under a
// `## Blocked by` heading up to the next heading (the to-tickets template).
func blockers(body string) []int {
	var refs []string
	inSection := false
	for line := range strings.Lines(body) {
		line = strings.TrimRight(line, "\r\n")
		heading := markdownHeading.MatchString(line)
		if heading {
			inSection = false
		}
		if m := blockedBy.FindStringSubmatch(line); m != nil {
			refs = append(refs, m[1])
			inSection = heading
			continue
		}
		if inSection {
			refs = append(refs, line)
		}
	}
	var numbers []int
	for _, ref := range refs {
		for _, m := range issueRef.FindAllStringSubmatch(ref, -1) {
			if n, err := strconv.Atoi(m[1]); err == nil {
				numbers = append(numbers, n)
			}
		}
	}
	return numbers
}

func waives(body, number string) bool {
	waiver := regexp.MustCompile(`(?im)^\W*without ADR ` + number + `\s*:\s*\S`)
	return waiver.MatchString(body)
}

// Comment is what the tracker says on a ticket the check took out of the agents' queue.
func Comment(gaps []Gap) string {
	var b bytes.Buffer
	b.WriteString("This ticket builds on decisions that are not built yet, so it is back in triage:\n\n")
	for _, gap := range gaps {
		fmt.Fprintf(&b, "- %s\n", gap)
	}
	b.WriteString("\nFor each of them, either wait for a ticket that builds it (`Blocked by #<n>`, or a native " +
		"blocking edge; file one and add its number to the ADR's `build:` when there is none), or say why this " +
		"ticket builds without it on a line `Without ADR <NNNN>: <why>`. Then label it `ready-for-agent` again; " +
		"`mise run tickets:check -- <number>` runs the same check. This is not a request for `/triage`: the ticket " +
		"is already written, so add the edges or waivers, or slice it again with `/to-tickets`.\n")
	return b.String()
}
