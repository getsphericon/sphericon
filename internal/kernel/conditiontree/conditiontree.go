// Package conditiontree is a Condition as it is stored (ADR 0169): the root Group of the tree of
// tests on a Contact, the type of the document the Condition columns hold. Each place a Condition
// is used has a type of its own over the same document, since each admits its own leaves (ADR
// 0181): a Segment's (Segment), a Rule's (Rule), a Branch path's (Branch) and an Event matcher's
// filter (EventFilter).
// internal/domain/contacts/condition decodes the tree of each place and compiles a Segment's and a
// Rule's to a Contact predicate, a filter to an SQL/JSON path (ADR 0180). Its vocabulary (the kinds of entry, test
// and source each place and leaf admits, the operators, the attributes) is generated from the
// contract.
package conditiontree

import "encoding/json"

// Group is a group of the tree. Each entries[] item is decoded lazily because it is either a
// nested Group or a leaf, as its `kind` says.
type Group struct {
	Kind       Kind              `json:"kind"`
	Combinator Combinator        `json:"combinator"`
	Not        bool              `json:"not,omitempty"`
	Entries    []json.RawMessage `json:"entries"`
}

// Segment is a Segment's Condition, and the copy a scheduled Broadcast keeps of it: its leaves
// test the Contact.
type Segment struct{ Group }

// Rule is a Rule's Condition: a Segment's leaves, and tests of the triggering Event's properties.
type Rule struct{ Group }

// Branch is the Condition of a Branch's path (ADR 0124): a Rule's leaves, its property tests reading
// the Event the Enrollment began with.
type Branch struct{ Group }

// EventFilter is an Event matcher's filter (ADR 0133, ADR 0180): property tests of the one Event
// it is about, which name no subject, under groups.
type EventFilter struct{ Group }

// Place is the type of a Condition of one place.
type Place interface {
	Segment | Rule | Branch | EventFilter
}

// Empty is the Condition with no tests, which matches every Contact.
func Empty() Group {
	return Group{Kind: KindGroup, Combinator: And, Entries: []json.RawMessage{}}
}

// Size counts the nodes of the group: itself and every nested group and leaf, at any depth, and the
// nodes of an event leaf's filter, so a filter does not hide tests from the bound on a Condition's
// size. A malformed entry counts as one node.
func (g Group) Size() int {
	n := 1
	for _, raw := range g.Entries {
		var sub Group
		if json.Unmarshal(raw, &sub) == nil && sub.Kind == KindGroup {
			n += sub.Size()
			continue
		}
		n++
		var leaf struct {
			Filter *Group `json:"filter"`
		}
		if json.Unmarshal(raw, &leaf) == nil && leaf.Filter != nil {
			n += leaf.Filter.Size()
		}
	}
	return n
}

// Truth is what a Condition is for a Contact by Kleene's logic (ADR 0123, ADR 0124): it holds, it
// fails, or it is unknown because an engagement count it reads is unknown for the Contact. A
// Branch's Step record keeps each path's.
type Truth string

// The three truths.
const (
	TruthTrue    Truth = "true"
	TruthFalse   Truth = "false"
	TruthUnknown Truth = "unknown"
)

// Tests reports whether the group holds a test, at any depth: a leaf, or an entry that is no group.
// A group of empty groups tests nothing.
func (g Group) Tests() bool {
	for _, raw := range g.Entries {
		var sub Group
		if json.Unmarshal(raw, &sub) == nil && sub.Kind == KindGroup {
			if sub.Tests() {
				return true
			}
			continue
		}
		return true
	}
	return false
}
