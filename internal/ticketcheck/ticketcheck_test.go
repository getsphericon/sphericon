package ticketcheck_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/ticketcheck"
)

func adr(t *testing.T, name, text string) ticketcheck.ADR {
	t.Helper()
	parsed, err := ticketcheck.ParseADR("docs/adr/"+name, text)
	require.NoError(t, err)
	return parsed
}

func TestParseADRReadsTheBuildField(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want ticketcheck.Build
	}{
		"absent":        {"---\nstatus: accepted\n---\n\n# Title\n", ticketcheck.Build{}},
		"no front":      {"# Title\n", ticketcheck.Build{}},
		"none":          {"---\nstatus: accepted\nbuild: none\n---\n", ticketcheck.Build{Recorded: true}},
		"done":          {"---\nstatus: accepted\nbuild: done\n---\n", ticketcheck.Build{Recorded: true, Done: true}},
		"tickets":       {"---\nstatus: accepted\nbuild: [1456, 1457]\n---\n", ticketcheck.Build{Recorded: true, Tickets: []int{1456, 1457}}},
		"free status":   {"---\nstatus: superseded in part by ADR 0101 (x: y)\nbuild: [7]\n---\n", ticketcheck.Build{Recorded: true, Tickets: []int{7}}},
		"windows lines": {"---\r\nbuild: none\r\n---\r\n", ticketcheck.Build{Recorded: true}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, adr(t, "0001-x.md", tc.text).Build)
		})
	}
}

func TestParseADRRefusesAnythingElse(t *testing.T) {
	for name, tc := range map[string]struct{ name, text, want string }{
		"yes":           {"0001-x.md", "---\nbuild: yes\n---\n", "`build:` is `none`"},
		"empty list":    {"0001-x.md", "---\nbuild: []\n---\n", "`build:` is `none`"},
		"hash in list":  {"0001-x.md", "---\nbuild: ['#12']\n---\n", "`build:` is `none`"},
		"bare hash":     {"0001-x.md", "---\nbuild: #12\n---\n", "`build:` is `none`"},
		"zero":          {"0001-x.md", "---\nbuild: [0]\n---\n", "`build:` is `none`"},
		"bad file name": {"x-0001.md", "---\nbuild: none\n---\n", "<four digits>-<title>.md"},
		"open list":     {"0001-x.md", "---\nbuild: [12\n---\n", "`build:` is `none`"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ticketcheck.ParseADR(tc.name, tc.text)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestCheckADRsRequiresTheFieldFromRecordedFromOn(t *testing.T) {
	older := adr(t, "0177-older.md", "---\nstatus: accepted\n---\n")
	recorded := adr(t, "0178-recorded.md", "---\nstatus: accepted\nbuild: none\n---\n")
	require.NoError(t, ticketcheck.CheckADRs([]ticketcheck.ADR{older, recorded}))

	missing := adr(t, "0179-missing.md", "---\nstatus: accepted\n---\n")
	require.ErrorContains(t, ticketcheck.CheckADRs([]ticketcheck.ADR{older, missing}), "ADR 0179 has no `build:`")
}

func TestReferencesNamesEveryADRATextNames(t *testing.T) {
	text := "Following ADR 0136, amending ADRs 0054, 0122 and 0124; ADR-0007; see [[0080-a-broadcast]] " +
		"and docs/adr/0139-skills.md. Not 1234 alone, nor #1456."
	require.Equal(t, []string{"0007", "0054", "0080", "0122", "0124", "0136", "0139"}, ticketcheck.References(text))
}

// The decisions of the checks below: 0136 is not built and no ticket builds it, 0126 is built by an
// open ticket, 0120 by a closed one, 0080 says nothing (built), 0149 says done, and 0161 is the
// decision the ticket under test builds.
func decisions(t *testing.T) []ticketcheck.ADR {
	t.Helper()
	return []ticketcheck.ADR{
		adr(t, "0080-one-channel.md", "---\nstatus: accepted\n---\n"),
		adr(t, "0120-tracking.md", "---\nbuild: [1300]\n---\n"),
		adr(t, "0126-wait-until.md", "---\nbuild: [1400]\n---\n"),
		adr(t, "0136-awaited.md", "---\nbuild: none\n---\n"),
		adr(t, "0149-audience.md", "---\nbuild: done\n---\n"),
		adr(t, "0161-http-response.md", "---\nbuild: [1456]\n---\n\n# HTTP response\n\nFollowing [[0136-awaited]].\n"),
		adr(t, "0170-amended.md", "---\nbuild: [1470]\n---\n\n> Amended by [[0171-later]].\n"),
		adr(t, "0171-later.md", "---\nbuild: none\n---\n"),
	}
}

// tracked says that its open numbers are open, 1301 was closed as not planned, and every other
// ticket was completed; slices gives a spec's sub-issues.
type tracked struct {
	open   []int
	slices map[int][]int
}

func (t tracked) State(n int) ticketcheck.TicketState {
	switch {
	case slices.Contains(t.open, n):
		return ticketcheck.Open
	case n == 1301:
		return ticketcheck.NotPlanned
	default:
		return ticketcheck.Completed
	}
}

func (t tracked) SubIssues(n int) []int { return t.slices[n] }

func openTickets(numbers ...int) tracked { return tracked{open: numbers} }

func TestCheckFindsAnUnbuiltADRTheTicketNames(t *testing.T) {
	issue := ticketcheck.Issue{Number: 1476, Title: "Window form of Wait until", Body: "Extends ADR 0126.\n"}
	gaps := ticketcheck.Check(issue, decisions(t), openTickets(1400))
	require.Equal(t, []ticketcheck.Gap{{ADR: "0126", Tickets: []int{1400}}}, gaps)
	require.Equal(t, "ADR 0126 is not built yet: #1400 builds it", gaps[0].String())
}

func TestCheckReadsOnlyTheADRsTheTicketNames(t *testing.T) {
	// ADR 0161 builds on 0136, which is not built, but a ticket that names only 0161 passes: the check
	// reads what the ticket names, not what its ADR names. Naming 0136 brings the gap back.
	quiet := ticketcheck.Issue{Number: 1456, Title: "Keep the response (ADR 0161)", Body: "See the ADR.\n"}
	require.Empty(t, ticketcheck.Check(quiet, decisions(t), openTickets(1456)))

	named := ticketcheck.Issue{Number: 1456, Title: "Keep the response (ADR 0161)", Body: "Follows ADR 0136.\n"}
	require.Equal(t, []ticketcheck.Gap{{ADR: "0136"}}, ticketcheck.Check(named, decisions(t), openTickets(1456)))
}

func TestCheckCountsATicketClosedAsNotPlannedAsNoTicket(t *testing.T) {
	adrs := append(decisions(t), adr(t, "0165-key.md", "---\nbuild: [1301]\n---\n"))
	issue := ticketcheck.Issue{Number: 1, Body: "Extends ADR 0165.\n"}
	require.Equal(t, []ticketcheck.Gap{{ADR: "0165"}}, ticketcheck.Check(issue, adrs, openTickets()))
}

func TestCheckTakesTheADRsOfATitleListAndOfAParentSpecAsItsOwn(t *testing.T) {
	// 0126 and 0136 are built by #1500, a spec; its sub-issue and a ticket whose title lists both
	// build them, so neither waits for the other.
	adrs := append(decisions(t),
		adr(t, "0126-wait-until.md", "---\nbuild: [1500]\n---\n\nAlso [[0136-awaited]].\n"),
		adr(t, "0136-awaited.md", "---\nbuild: [1500]\n---\n"))
	adrs = slices.DeleteFunc(adrs, func(a ticketcheck.ADR) bool {
		return (a.Number == "0126" || a.Number == "0136") && !slices.Contains(a.Build.Tickets, 1500)
	})
	for name, issue := range map[string]ticketcheck.Issue{
		"sub-issue":  {Number: 1501, Parent: 1500, Title: "Wait until", Body: "ADR 0126 and ADR 0136.\n"},
		"title list": {Number: 1502, Title: "Both (ADR 0126, 0136)", Body: "ADRs 0126 and 0136.\n"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Empty(t, ticketcheck.Check(issue, adrs, openTickets(1500, 1501, 1502)))
		})
	}
}

func TestCheckTakesASliceOfTheSpecAnADRListsAsAWait(t *testing.T) {
	// ADR 0133 is built by the spec #1546, sliced into #1549 (done) and #1551 (open); ADR 0134 by
	// #1300 (done) and #1400 (open).
	adrs := append(decisions(t),
		adr(t, "0133-matchers.md", "---\nbuild: [1546]\n---\n"),
		adr(t, "0134-window.md", "---\nbuild: [1300, 1400]\n---\n"))
	spec := tracked{open: []int{1400, 1546, 1551, 1600}, slices: map[int][]int{1546: {1549, 1551}}}
	for name, issue := range map[string]ticketcheck.Issue{
		"the open slice it needs": {Number: 1, Body: "Filters as ADR 0133.\n", BlockedBy: []int{1551}},
		"a slice already done":    {Number: 1, Body: "Filters as ADR 0133.\n", BlockedBy: []int{1549}},
		"the whole spec":          {Number: 1, Body: "Filters as ADR 0133.\n", BlockedBy: []int{1546}},
		"a listed ticket done":    {Number: 1, Body: "Uses ADR 0134.\n", BlockedBy: []int{1300}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Empty(t, ticketcheck.Check(issue, adrs, spec))
		})
	}

	// A slice closed as not planned (#1301) builds nothing, and another ticket is no wait; the gap
	// names the spec and its open slices.
	for name, issue := range map[string]ticketcheck.Issue{
		"a slice not planned": {Number: 1, Body: "Filters as ADR 0133.\n", BlockedBy: []int{1301}},
		"another ticket":      {Number: 1, Body: "Filters as ADR 0133.\n", BlockedBy: []int{1600}},
	} {
		t.Run(name, func(t *testing.T) {
			withDropped := tracked{open: spec.open, slices: map[int][]int{1546: {1549, 1551, 1301}}}
			gaps := ticketcheck.Check(issue, adrs, withDropped)
			require.Equal(t, []ticketcheck.Gap{{ADR: "0133", Tickets: []int{1546}, Slices: []int{1551}}}, gaps)
			require.Equal(t, "ADR 0133 is not built yet: #1546 builds it (wait for the slice this ticket needs: #1551)",
				gaps[0].String())
		})
	}
}

func TestCheckPassesWhatIsBuiltWaitedForOrWaived(t *testing.T) {
	for name, issue := range map[string]ticketcheck.Issue{
		"built: no field, done, closed ticket": {Number: 1, Body: "Uses ADRs 0080, 0149 and 0120.\n"},
		"blocked by line":                      {Number: 1, Body: "Blocked by #1438 (the ADR) and #1400.\n\nExtends ADR 0126.\n"},
		"native edge":                          {Number: 1, Body: "Extends ADR 0126.\n", BlockedBy: []int{1400}},
		"blocked by heading":                   {Number: 1, Body: "Extends ADR 0126.\n\n## Blocked by\n\n- #1438\n- #1400\n\n## Notes\n"},
		"waived":                               {Number: 1, Body: "Extends ADR 0136.\n\nWithout ADR 0136: the copy is read only when it exists.\n"},
		"only its own ADR":                     {Number: 1, Title: "Wait until (ADR 0126)", Body: "ADR 0126.\n"},
		"an ADR not on main yet":               {Number: 1, Body: "Blocked by #1475 (the ADR). ADR 0172.\n"},
		"a later ADR its own ADR names":        {Number: 1470, Title: "Amended (ADR 0170)", Body: "See the ADR.\n"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Empty(t, ticketcheck.Check(issue, decisions(t), openTickets(1400)))
		})
	}
}

func TestCheckHoldsWhatIsNeitherWaitedForNorWaived(t *testing.T) {
	for name, issue := range map[string]ticketcheck.Issue{
		"blocked by another ticket":            {Number: 1, Body: "Blocked by #1438.\n\nExtends ADR 0126.\n"},
		"a waiver of another ADR":              {Number: 1, Body: "Extends ADR 0126.\n\nWithout ADR 0136: not needed.\n"},
		"a waiver without a reason":            {Number: 1, Body: "Extends ADR 0126.\n\nWithout ADR 0126:\n"},
		"a ticket named in prose":              {Number: 1, Body: "Extends ADR 0126, which #1400 builds.\n"},
		"a ticket after the heading's section": {Number: 1, Body: "## Blocked by\n\n- #1438\n\n## Notes\n\nExtends ADR 0126 (#1400).\n"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, []ticketcheck.Gap{{ADR: "0126", Tickets: []int{1400}}},
				ticketcheck.Check(issue, decisions(t), openTickets(1400)))
		})
	}
}

func TestReviewWantsANativeEdgeForEachOpenIssueABlockedByLineNames(t *testing.T) {
	// #1400 is open; #1438 is closed, or a pull request, which the tracker cannot gate on.
	for name, tc := range map[string]struct {
		issue ticketcheck.Issue
		want  []int
	}{
		"a line, no edge":      {ticketcheck.Issue{Number: 1, Body: "Blocked by #1400 and #1438.\n"}, []int{1400}},
		"a section, no edge":   {ticketcheck.Issue{Number: 1, Body: "## Blocked by\n\n- #1400\n- #1400\n"}, []int{1400}},
		"a line with its edge": {ticketcheck.Issue{Number: 1, Body: "Blocked by #1400.\n", BlockedBy: []int{1400}}, nil},
		"the parent's edge":    {ticketcheck.Issue{Number: 1, Parent: 9, Body: "Blocked by #1400.\n", BlockedBy: []int{1400}}, nil},
		"a ticket in prose":    {ticketcheck.Issue{Number: 1, Body: "Needs what #1400 builds.\n"}, nil},
		"a closed issue":       {ticketcheck.Issue{Number: 1, Body: "Blocked by #1438 (the ADR).\n"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			report := ticketcheck.Review(tc.issue, decisions(t), openTickets(1400))
			require.Equal(t, tc.want, report.Unlinked)
		})
	}
}

func TestReviewHoldsASpecUntilItHasSlices(t *testing.T) {
	spec := "## Problem Statement\n\nx\n\n## User Stories\n\n1. As a marketer\n\n## Implementation Decisions\n\n- y\n"
	for name, tc := range map[string]struct {
		issue ticketcheck.Issue
		want  bool
	}{
		"a spec with no slices": {ticketcheck.Issue{Number: 1, Body: spec}, true},
		"a sliced spec":         {ticketcheck.Issue{Number: 1, Body: spec, SubIssues: []int{2, 3}}, false},
		"a ticket":              {ticketcheck.Issue{Number: 1, Body: "## What to build\n\n## User Stories\n"}, false},
	} {
		t.Run(name, func(t *testing.T) {
			report := ticketcheck.Review(tc.issue, decisions(t), openTickets())
			require.Equal(t, tc.want, report.Unsliced)
			require.Equal(t, tc.want, report.Failed())
		})
	}
}

func TestCommentNamesEveryFindingAndTheWayOut(t *testing.T) {
	comment := ticketcheck.Comment(ticketcheck.Report{
		Gaps:     []ticketcheck.Gap{{ADR: "0136"}, {ADR: "0126", Tickets: []int{1400}}},
		Unlinked: []int{1400, 1401},
		Unsliced: true,
	})
	require.Contains(t, comment, "- ADR 0136 is not built, and no ticket builds it yet\n")
	require.Contains(t, comment, "- ADR 0126 is not built yet: #1400 builds it\n")
	require.Contains(t, comment, "- the body says it is blocked by #1400, #1401, with no native blocking edge\n")
	require.Contains(t, comment, "- a spec with no slices: no sub-issue builds it yet\n")
	require.Contains(t, comment, "`Without ADR <NNNN>: <why>`")
	require.Contains(t, comment, "`/to-tickets`")
	require.Contains(t, comment, "not a request for `/triage`")

	gapsOnly := ticketcheck.Comment(ticketcheck.Report{Gaps: []ticketcheck.Gap{{ADR: "0136"}}})
	require.NotContains(t, gapsOnly, "`/to-tickets`")
	require.NotContains(t, gapsOnly, "native blocking edge for each issue")
}
