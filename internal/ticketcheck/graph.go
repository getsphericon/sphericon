package ticketcheck

import (
	"fmt"
	"slices"
	"strings"
)

// Node is an open ticket with its native edges to other open tickets.
type Node struct {
	Number    int
	Title     string
	Labels    []string
	Parent    int   // the issue it is a sub-issue of, or 0
	BlockedBy []int // its open blockers
	SubIssues []int // its open sub-issues
}

// Row is what the graph says of one open ticket.
type Row struct {
	Node

	Waits     int         // the open tickets it waits for, its blockers' blockers included
	OnSpec    []int       // blockers with open sub-issues: a wait on a whole spec
	Redundant map[int]int // a blocker another blocker already waits for, with that other blocker
	Cycle     bool        // it waits for itself
}

// Graph reads the native edges between open tickets: how many tickets each waits for, which of its
// edges point at a whole spec rather than the slice it needs, and which say again what another
// edge already says. The rows come longest wait first.
func Graph(nodes []Node) []Row {
	byNumber := make(map[int]Node, len(nodes))
	for _, n := range nodes {
		byNumber[n.Number] = n
	}
	closure := func(from int) map[int]bool {
		seen := map[int]bool{}
		stack := slices.Clone(byNumber[from].BlockedBy)
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			stack = append(stack, byNumber[n].BlockedBy...)
		}
		return seen
	}
	rows := make([]Row, 0, len(nodes))
	for _, n := range nodes {
		waits := closure(n.Number)
		row := Row{Node: n, Waits: len(waits), Cycle: waits[n.Number], Redundant: map[int]int{}}
		if row.Cycle {
			row.Waits--
		}
		for _, b := range n.BlockedBy {
			if len(byNumber[b].SubIssues) > 0 {
				row.OnSpec = append(row.OnSpec, b)
			}
			for _, other := range n.BlockedBy {
				if other != b && closure(other)[b] {
					row.Redundant[b] = other
					break
				}
			}
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b Row) int {
		if a.Waits != b.Waits {
			return b.Waits - a.Waits
		}
		return b.Number - a.Number
	})
	return rows
}

// String is the row as `tickets:graph` prints it.
func (r Row) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d", r.Number)
	if len(r.Labels) > 0 {
		fmt.Fprintf(&b, " [%s]", strings.Join(r.Labels, ", "))
	}
	if r.Waits == 0 {
		b.WriteString(" startable")
	} else {
		fmt.Fprintf(&b, " waits for %d", r.Waits)
	}
	fmt.Fprintf(&b, ": %s", r.Title)
	if r.Parent != 0 {
		fmt.Fprintf(&b, "\n  part of #%d", r.Parent)
	}
	if r.Cycle {
		b.WriteString("\n  in a cycle: it waits for itself")
	}
	if len(r.BlockedBy) > 0 {
		blockers := make([]string, len(r.BlockedBy))
		for i, n := range r.BlockedBy {
			blockers[i] = fmt.Sprintf("#%d", n)
			if slices.Contains(r.OnSpec, n) {
				blockers[i] += " (a spec)"
			}
			if other, ok := r.Redundant[n]; ok {
				blockers[i] += fmt.Sprintf(" (already behind #%d)", other)
			}
		}
		fmt.Fprintf(&b, "\n  blocked by %s", strings.Join(blockers, ", "))
	}
	return b.String()
}
