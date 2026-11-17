// Package conditiontree is a Condition as it is stored (ADR 0169): the root Group of the tree of
// tests on a Contact, the type of the document the Condition columns hold. internal/domain/contacts/condition
// validates the tree and compiles it to a Contact predicate.
package conditiontree

import "encoding/json"

// KindGroup is the kind of a group entry of the tree.
const KindGroup = "group"

// Group is a group of the rule tree. Each rules[] entry is decoded lazily because it is either a
// nested Group or a leaf, as its `kind` says.
type Group struct {
	Kind       string            `json:"kind"`
	Combinator Combinator        `json:"combinator"`
	Not        bool              `json:"not,omitempty"`
	Rules      []json.RawMessage `json:"rules"`
}

// Empty is the Condition with no tests, which matches every Contact.
func Empty() Group {
	return Group{Kind: KindGroup, Combinator: And, Rules: []json.RawMessage{}}
}

// Size counts the nodes of the group: itself and every nested group and rule, at any depth. A
// malformed entry counts as one node.
func (g Group) Size() int {
	n := 1
	for _, raw := range g.Rules {
		var sub Group
		if json.Unmarshal(raw, &sub) == nil && sub.Kind == KindGroup {
			n += sub.Size()
			continue
		}
		n++
	}
	return n
}
