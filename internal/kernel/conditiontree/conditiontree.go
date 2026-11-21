// Package conditiontree is a Condition as it is stored (ADR 0169): the root Group of the tree of
// tests on a Contact, the type of the document the Condition columns hold. Each place a Condition
// is used has a type of its own over the same document, since each admits its own leaves (ADR
// 0181): a Segment's (Segment) and a Rule's (Rule). internal/domain/contacts/condition decodes the
// tree of each place and compiles it to a Contact predicate.
package conditiontree

import "encoding/json"

// KindGroup is the kind of a group entry of the tree.
const KindGroup = "group"

// Group is a group of the tree. Each entries[] item is decoded lazily because it is either a
// nested Group or a leaf, as its `kind` says.
type Group struct {
	Kind       string            `json:"kind"`
	Combinator Combinator        `json:"combinator"`
	Not        bool              `json:"not,omitempty"`
	Entries    []json.RawMessage `json:"entries"`
}

// Segment is a Segment's Condition, and the copy a scheduled Broadcast keeps of it: its leaves
// test the Contact.
type Segment struct{ Group }

// Rule is a Rule's Condition: a Segment's leaves, and tests of the triggering Event's properties.
type Rule struct{ Group }

// Place is the type of a Condition of one place.
type Place interface{ Segment | Rule }

// Empty is the Condition with no tests, which matches every Contact.
func Empty() Group {
	return Group{Kind: KindGroup, Combinator: And, Entries: []json.RawMessage{}}
}

// Size counts the nodes of the group: itself and every nested group and leaf, at any depth. A
// malformed entry counts as one node.
func (g Group) Size() int {
	n := 1
	for _, raw := range g.Entries {
		var sub Group
		if json.Unmarshal(raw, &sub) == nil && sub.Kind == KindGroup {
			n += sub.Size()
			continue
		}
		n++
	}
	return n
}
