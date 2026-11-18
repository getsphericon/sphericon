package ticketcheck_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/ticketcheck"
)

func TestGraphCountsWaitsAndNamesSpecAndRedundantEdges(t *testing.T) {
	// #10 is a spec sliced into #11 and #12, and #12 waits for #11. #20 waits for the spec, for #11
	// and for #12: the edge to the spec is a wait on a whole spec, and the edge to #11 is redundant,
	// since #12 already waits for it.
	nodes := []ticketcheck.Node{
		{Number: 10, Title: "Spec", SubIssues: []int{11, 12}},
		{Number: 11, Title: "Slice one", Parent: 10},
		{Number: 12, Title: "Slice two", Parent: 10, BlockedBy: []int{11}},
		{Number: 20, Title: "Uses it", Labels: []string{"ready-for-agent"}, BlockedBy: []int{10, 11, 12}},
	}
	rows := ticketcheck.Graph(nodes)
	require.Equal(t, []int{20, 12, 11, 10}, []int{rows[0].Number, rows[1].Number, rows[2].Number, rows[3].Number})

	uses := rows[0]
	require.Equal(t, 3, uses.Waits)
	require.Equal(t, []int{10}, uses.OnSpec)
	require.Equal(t, map[int]int{11: 12}, uses.Redundant)
	require.False(t, uses.Cycle)
	require.Equal(t, "#20 [ready-for-agent] waits for 3: Uses it\n"+
		"  blocked by #10 (a spec), #11 (already behind #12), #12", uses.String())

	require.Equal(t, "#11 startable: Slice one\n  part of #10", rows[2].String())
}

func TestGraphFindsACycle(t *testing.T) {
	rows := ticketcheck.Graph([]ticketcheck.Node{
		{Number: 1, Title: "A", BlockedBy: []int{2}},
		{Number: 2, Title: "B", BlockedBy: []int{1}},
	})
	for _, row := range rows {
		require.True(t, row.Cycle)
		require.Equal(t, 1, row.Waits)
		require.Contains(t, row.String(), "in a cycle")
	}
}
