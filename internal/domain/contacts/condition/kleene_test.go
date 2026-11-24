package condition_test

import (
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// truth is a value of Kleene's three-valued logic.
type truth int

const (
	unknown truth = iota
	no
	yes
)

// and is Kleene's conjunction: false if any side is, true if both are, unknown otherwise.
func (a truth) and(b truth) truth {
	switch {
	case a == no || b == no:
		return no
	case a == yes && b == yes:
		return yes
	}
	return unknown
}

// or is Kleene's disjunction: true if any side is, false if both are, unknown otherwise.
func (a truth) or(b truth) truth {
	switch {
	case a == yes || b == yes:
		return yes
	case a == no && b == no:
		return no
	}
	return unknown
}

// not keeps unknown.
func (a truth) not() truth {
	switch a {
	case yes:
		return no
	case no:
		return yes
	case unknown:
	}
	return unknown
}

// kleeneLeaf is a leaf of the model and what it is for each of Massive's Contacts once Ann and Bob
// refused tracking: Ann opened nothing, Bob once, Cara twice.
type kleeneLeaf struct {
	entry leaf
	of    map[int64]truth
}

var kleeneLeaves = []kleeneLeaf{
	{attribute("first_name", text("eq", "Ann")), map[int64]truth{fixtures.MassiveAnnID: yes, fixtures.MassiveBobID: no, fixtures.MassiveCaraID: no}},
	{attribute("first_name", text("eq", "Cara")), map[int64]truth{fixtures.MassiveAnnID: no, fixtures.MassiveBobID: no, fixtures.MassiveCaraID: yes}},
	{eventLeaf(systemevent.EmailOpened, "atMost", 1, 0), map[int64]truth{fixtures.MassiveAnnID: unknown, fixtures.MassiveBobID: unknown, fixtures.MassiveCaraID: no}},
	{eventLeaf(systemevent.EmailOpened, "atLeast", 1, 90), map[int64]truth{fixtures.MassiveAnnID: unknown, fixtures.MassiveBobID: unknown, fixtures.MassiveCaraID: yes}},
}

// kleeneNode is a group of the model: its combinator, its negation and its entries, each a leaf or
// a nested group.
type kleeneNode struct {
	or, not bool
	leaves  []kleeneLeaf
	groups  []kleeneNode
}

// kleeneTree draws a random group of the model's leaves, nested up to three deep.
func kleeneTree(t *rapid.T, depth int) kleeneNode {
	n := kleeneNode{or: rapid.Bool().Draw(t, "or"), not: rapid.Bool().Draw(t, "not")}
	for range rapid.IntRange(1, 3).Draw(t, "entries") {
		if depth < 3 && rapid.Bool().Draw(t, "nest") {
			n.groups = append(n.groups, kleeneTree(t, depth+1))
			continue
		}
		n.leaves = append(n.leaves, rapid.SampledFrom(kleeneLeaves).Draw(t, "leaf"))
	}
	return n
}

// group is the node as the stored tree.
func (n kleeneNode) group() conditiontree.Group {
	entries := make([]any, 0, len(n.leaves)+len(n.groups))
	for _, l := range n.leaves {
		entries = append(entries, l.entry)
	}
	for _, g := range n.groups {
		entries = append(entries, g.group())
	}
	combinator := conditiontree.And
	if n.or {
		combinator = conditiontree.Or
	}
	return groupOf(combinator, n.not, entries...)
}

// of is the node's value for the Contact id by Kleene's rules.
func (n kleeneNode) of(id int64) truth {
	values := make([]truth, 0, len(n.leaves)+len(n.groups))
	for _, l := range n.leaves {
		values = append(values, l.of[id])
	}
	for _, g := range n.groups {
		values = append(values, g.of(id))
	}
	out := values[0]
	for _, v := range values[1:] {
		if n.or {
			out = out.or(v)
		} else {
			out = out.and(v)
		}
	}
	if n.not {
		out = out.not()
	}
	return out
}

// expected is what the model says of the Contacts ids: the ones it holds for are matched, the ones
// it is unknown for are left out.
func (n kleeneNode) expected(ids []int64) verdicts {
	var out verdicts
	for _, id := range ids {
		switch n.of(id) {
		case yes:
			out.matched = append(out.matched, id)
		case unknown:
			out.leftOut = append(out.leftOut, id)
		case no:
		}
	}
	return out
}
