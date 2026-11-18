// Package ticketcheck checks that a ticket ready for an agent builds on decisions that are built. An
// ADR says in its front matter whether it is built (`build:`). A ticket that names an ADR not built
// yet either waits for a ticket that builds it or says why it builds without it: an agent handed the
// ticket otherwise finds nothing to build on. Only the ADRs the ticket names count, not those its own
// ADR names, so a ticket names every ADR it builds on. An ADR no ticket builds yet may list the terms
// that name what it decides (`terms:`), and a ticket that uses one names that ADR as if by its number. Two more things keep a ticket from an agent:
// an open issue its `Blocked by` line names with no native edge, and, for a spec, no slices yet.
// `cmd/ticketcheck` runs the checks on the files of docs/adr and on the issues of the tracker.
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
	// Terms name what it decides and nothing built does, such as "Set field" or "Merge": while no
	// ticket builds it, a ticket that uses one builds on it.
	Terms []string
}

// Build is what an ADR's `build:` field says.
type Build struct {
	Recorded bool  // the field is there
	Done     bool  // `done`: built, or nothing to build
	Tickets  []int // the tickets that build it; empty with `none`
}

// ParseADR reads an ADR file's number from its name and its `build:` and `terms:` fields from its
// front matter.
func ParseADR(name, text string) (ADR, error) {
	base := filepath.Base(name)
	if len(base) < 5 || base[4] != '-' || strings.Trim(base[:4], "0123456789") != "" {
		return ADR{}, fmt.Errorf("%s: an ADR file is named <four digits>-<title>.md", name)
	}
	build, err := parseBuild(text)
	if err != nil {
		return ADR{}, fmt.Errorf("%s: %w", name, err)
	}
	terms, err := parseTerms(text)
	if err != nil {
		return ADR{}, fmt.Errorf("%s: %w", name, err)
	}
	return ADR{Number: base[:4], Text: text, Build: build, Terms: terms}, nil
}

var errTermsValue = errors.New("`terms:` is a list of the terms that name what the ADR decides, such as " +
	"`[Set field, Notify]`")

// parseTerms reads the `terms:` line of the front matter, a list of terms separated by commas.
func parseTerms(text string) ([]string, error) {
	front, ok := frontMatter(text)
	if !ok {
		return nil, nil
	}
	for line := range strings.Lines(front) {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "terms:")
		if !ok {
			continue
		}
		list, ok := strings.CutPrefix(strings.TrimSpace(value), "[")
		list, closed := strings.CutSuffix(list, "]")
		if !ok || !closed {
			return nil, errTermsValue
		}
		var terms []string
		for item := range strings.SplitSeq(list, ",") {
			term := strings.TrimSpace(item)
			if term == "" || strings.ContainsAny(term, "[]#`") {
				return nil, errTermsValue
			}
			terms = append(terms, term)
		}
		return terms, nil
	}
	return nil, nil
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
	Parent    int   // the issue it is a sub-issue of, or 0
	BlockedBy []int // the tracker's native blocking edges, the parent's included
	SubIssues []int // its own sub-issues, whatever their state
}

// Tickets is how the tracker's tickets stand.
type Tickets interface {
	State(number int) TicketState
	// SubIssues returns the sub-issues of a ticket, whatever their state: the slices of a spec.
	SubIssues(number int) []int
}

// Gap is an ADR a ticket builds on that is not built, with the open tickets its `build:` lists and
// their open slices, and the ADR's terms the ticket uses when it does not name the ADR by number.
type Gap struct {
	ADR     string
	Tickets []int
	Slices  []int
	Terms   []string
}

func (g Gap) String() string {
	adr := "ADR " + g.ADR
	if len(g.Terms) > 0 {
		adr += " (named by " + quoted(g.Terms) + ")"
	}
	if len(g.Tickets) == 0 {
		return adr + " is not built, and no ticket builds it yet"
	}
	text := fmt.Sprintf("%s is not built yet: %s builds it", adr, refs(g.Tickets))
	if len(g.Slices) > 0 {
		text += fmt.Sprintf(" (wait for the slice this ticket needs: %s)", refs(g.Slices))
	}
	return text
}

func quoted(terms []string) string {
	out := make([]string, len(terms))
	for i, term := range terms {
		out[i] = strconv.Quote(term)
	}
	return strings.Join(out, ", ")
}

func refs(numbers []int) string {
	out := make([]string, len(numbers))
	for i, n := range numbers {
		out[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(out, ", ")
}

var (
	// "ADR 0136", "ADRs 0122, 0124 and 0125", "ADR-0007".
	adrList         = regexp.MustCompile(`\bADRs?[\s-]+(\d{4}(?:(?:\s*,\s*|\s*,?\s+and\s+)\d{4})*)`)
	adrLink         = regexp.MustCompile(`(?:\[\[|docs/adr/)(\d{4})-`)
	fourDigit       = regexp.MustCompile(`\d{4}`)
	ownADR          = regexp.MustCompile(`\(ADRs? (\d{4}(?:(?:\s*,\s*|\s*,?\s+and\s+)\d{4})*)\)`)
	issueRef        = regexp.MustCompile(`#(\d+)`)
	blockedBy       = regexp.MustCompile(`(?i)^\W*blocked by\b(.*)$`)
	markdownHeading = regexp.MustCompile(`^\s*#{1,6}\s`)
	anyWaiver       = regexp.MustCompile(`(?i)^\W*without ADR \d{4}\s*:`)
)

// usedTerms returns the terms of an ADR no ticket builds yet that a text uses, as written (the case
// counts, so "merge tag" is not "Merge"), a plural `s` allowed. A waiver line does not count: it says
// what the ticket builds without.
func usedTerms(adr ADR, text string) []string {
	if !adr.Build.Recorded || adr.Build.Done || len(adr.Build.Tickets) > 0 {
		return nil
	}
	var lines []string
	for line := range strings.Lines(text) {
		if !anyWaiver.MatchString(line) {
			lines = append(lines, line)
		}
	}
	kept := strings.Join(lines, "")
	var used []string
	for _, term := range adr.Terms {
		word := regexp.MustCompile(`(?:^|[^\pL\pN_-])` + regexp.QuoteMeta(term) + `s?(?:$|[^\pL\pN_-])`)
		if word.MatchString(kept) {
			used = append(used, term)
		}
	}
	return used
}

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

// TicketState is how a ticket an ADR's `build:` lists stands.
type TicketState int

const (
	// Open: the ticket is still building the ADR.
	Open TicketState = iota
	// Completed: the ticket was closed as done.
	Completed
	// NotPlanned: the ticket was closed without being built (superseded, a duplicate); it builds
	// nothing.
	NotPlanned
)

// Check returns the ADRs the issue builds on that are not built and that it neither waits for nor
// waives. The issue builds on every ADR its title and body name but its own: those its title names
// as `(ADR NNNN)` or `(ADR NNNN, NNNN)` and those whose `build:` lists it or its parent (a sub-issue
// of a spec builds what the spec builds). An ADR is built when it has no `build:` field, has `done`,
// or lists no open ticket and at least one completed one. The issue waits for an ADR when it is
// blocked (a native edge, its parent's, or a `Blocked by` line) by a ticket that builds it, open or
// completed: one its `build:` lists, or a slice of one, so a ticket waits for the slice of a spec it
// needs rather than the whole spec, and a slice that is done still counts. It waives the ADR with a
// line `Without ADR NNNN: <why>`. A ticket names an ADR no ticket builds yet by its number or by one of
// its terms.
func Check(issue Issue, adrs []ADR, tickets Tickets) []Gap {
	byNumber := make(map[string]ADR, len(adrs))
	own := map[string]bool{}
	for _, adr := range adrs {
		byNumber[adr.Number] = adr
		if slices.Contains(adr.Build.Tickets, issue.Number) ||
			(issue.Parent != 0 && slices.Contains(adr.Build.Tickets, issue.Parent)) {
			own[adr.Number] = true
		}
	}
	for _, m := range ownADR.FindAllStringSubmatch(issue.Title, -1) {
		for _, number := range fourDigit.FindAllString(m[1], -1) {
			own[number] = true
		}
	}

	text := issue.Title + "\n" + issue.Body
	refs := References(text)
	terms := map[string][]string{}
	for _, adr := range adrs {
		if used := usedTerms(adr, text); len(used) > 0 && !slices.Contains(refs, adr.Number) {
			terms[adr.Number] = used
			refs = append(refs, adr.Number)
		}
	}
	slices.Sort(refs)
	waits := append(slices.Clone(issue.BlockedBy), blockers(issue.Body)...)

	var gaps []Gap
	for _, number := range refs {
		adr, ok := byNumber[number]
		if own[number] || !ok || !adr.Build.Recorded || adr.Build.Done {
			continue
		}
		var building, open []int
		completed := false
		for _, n := range adr.Build.Tickets {
			switch tickets.State(n) {
			case Open:
				open = append(open, n)
			case Completed:
				completed = true
			case NotPlanned:
				continue
			}
			building = append(building, n)
			building = append(building, tickets.SubIssues(n)...)
		}
		if len(open) == 0 && completed {
			continue
		}
		waited := func(n int) bool { return slices.Contains(waits, n) && tickets.State(n) != NotPlanned }
		if slices.ContainsFunc(building, waited) || waives(issue.Body, number) {
			continue
		}
		var slicesOpen []int
		for _, n := range building {
			if tickets.State(n) == Open && !slices.Contains(open, n) {
				slicesOpen = append(slicesOpen, n)
			}
		}
		gaps = append(gaps, Gap{ADR: number, Tickets: open, Slices: slicesOpen, Terms: terms[number]})
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

// Report is what keeps a ticket from an agent: the unbuilt ADRs it builds on, the open issues its
// body says it is blocked by with no native edge, and, for a spec, that it has no slices yet.
type Report struct {
	Gaps     []Gap
	Unlinked []int
	Unsliced bool
}

// Review checks a ticket ready for an agent. A `Blocked by` line or section is the to-tickets
// template's mirror of the native edges, so an open issue it names must be one of them: the tracker
// gates on the edges, and a line alone gates nothing. A spec (the to-spec template: its `## User
// Stories` and `## Implementation Decisions`) is ready once to-tickets has sliced it into sub-issues.
func Review(issue Issue, adrs []ADR, tickets Tickets) Report {
	report := Report{Gaps: Check(issue, adrs, tickets)}
	for _, n := range blockers(issue.Body) {
		if n != issue.Number && tickets.State(n) == Open && !slices.Contains(issue.BlockedBy, n) &&
			!slices.Contains(report.Unlinked, n) {
			report.Unlinked = append(report.Unlinked, n)
		}
	}
	report.Unsliced = isSpec(issue.Body) && len(issue.SubIssues) == 0
	return report
}

// Failed reports whether anything keeps the ticket from an agent.
func (r Report) Failed() bool { return len(r.Gaps) > 0 || len(r.Unlinked) > 0 || r.Unsliced }

// Lines names each finding on a line of its own.
func (r Report) Lines() []string {
	var lines []string
	for _, gap := range r.Gaps {
		lines = append(lines, gap.String())
	}
	if len(r.Unlinked) > 0 {
		lines = append(lines, fmt.Sprintf("the body says it is blocked by %s, with no native blocking edge", refs(r.Unlinked)))
	}
	if r.Unsliced {
		lines = append(lines, "a spec with no slices: no sub-issue builds it yet")
	}
	return lines
}

var specHeadings = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^##\s+User Stories\s*$`),
	regexp.MustCompile(`(?m)^##\s+Implementation Decisions\s*$`),
}

func isSpec(body string) bool {
	for _, heading := range specHeadings {
		if !heading.MatchString(body) {
			return false
		}
	}
	return true
}

// Comment is what the tracker says on a ticket the check took out of the agents' queue.
func Comment(r Report) string {
	var b bytes.Buffer
	b.WriteString("This ticket is not ready for an agent yet, so it is back in triage:\n\n")
	for _, line := range r.Lines() {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	b.WriteString("\n")
	if len(r.Gaps) > 0 {
		b.WriteString("For each ADR, either wait for a ticket that builds it (a native blocking edge to it, or to the " +
			"slice of it this ticket needs; file one and add its number to the ADR's `build:` when there is none), or " +
			"say why this ticket builds without it on a line `Without ADR <NNNN>: <why>`.\n\n")
	}
	if len(r.Unlinked) > 0 {
		b.WriteString("Add a native blocking edge for each issue the `Blocked by` line names, or take it off the line " +
			"(`docs/agents/issue-tracker.md`, \"Blocking\").\n\n")
	}
	if r.Unsliced {
		b.WriteString("Slice the spec with `/to-tickets`; its sub-issues carry the label, and the spec gets it back " +
			"once they exist.\n\n")
	}
	b.WriteString("Then label it `ready-for-agent` again; `mise run tickets:check -- <number>` runs the same check. " +
		"This is not a request for `/triage`: the ticket is already written.\n")
	return b.String()
}
