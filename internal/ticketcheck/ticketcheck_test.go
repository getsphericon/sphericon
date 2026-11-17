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

func openTickets(numbers ...int) func(int) bool {
	return func(n int) bool { return slices.Contains(numbers, n) }
}

func TestCheckFindsAnUnbuiltADRTheTicketNames(t *testing.T) {
	issue := ticketcheck.Issue{Number: 1476, Title: "Window form of Wait until", Body: "Extends ADR 0126.\n"}
	gaps := ticketcheck.Check(issue, decisions(t), openTickets(1400))
	require.Equal(t, []ticketcheck.Gap{{ADR: "0126", Tickets: []int{1400}}}, gaps)
	require.Equal(t, "ADR 0126 is not built yet: #1400 builds it", gaps[0].String())
}

func TestCheckFindsAnUnbuiltADRTheTicketsOwnADRNames(t *testing.T) {
	// The body names only its own ADR, which builds on 0136: the gap is found through the ADR's text,
	// whether the ADR is the ticket's own by its title or by its `build:` list.
	byTitle := ticketcheck.Issue{Number: 9999, Title: "Keep the response (ADR 0161)", Body: "See the ADR.\n"}
	byList := ticketcheck.Issue{Number: 1456, Title: "Keep the response", Body: "See the ADR.\n"}
	for _, issue := range []ticketcheck.Issue{byTitle, byList} {
		require.Equal(t, []ticketcheck.Gap{{ADR: "0136"}}, ticketcheck.Check(issue, decisions(t), openTickets(1456)))
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

func TestCommentNamesEveryGapAndTheWayOut(t *testing.T) {
	comment := ticketcheck.Comment([]ticketcheck.Gap{{ADR: "0136"}, {ADR: "0126", Tickets: []int{1400}}})
	require.Contains(t, comment, "- ADR 0136 is not built, and no ticket builds it yet\n")
	require.Contains(t, comment, "- ADR 0126 is not built yet: #1400 builds it\n")
	require.Contains(t, comment, "`Without ADR <NNNN>: <why>`")
}
