package condition_test

import (
	"encoding/json"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// The builders of a stored tree (ADR 0181), so a test reads as the Condition it compiles. A leaf or
// a test is the JSON object the contract names; a nested group is a conditiontree.Group.

// leaf is an entry of a group, or a leaf's test, as its JSON object.
type leaf = map[string]any

// marshal encodes leaves (or nested groups) into the raw entries[] of a group.
func marshal(entries ...any) []json.RawMessage {
	out := make([]json.RawMessage, len(entries))
	for i, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			panic(err)
		}
		out[i] = b
	}
	return out
}

// groupOf is a group of entries combined by c, negated when not is set.
func groupOf(c conditiontree.Combinator, not bool, entries ...any) conditiontree.Group {
	return conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: c, Not: not, Entries: marshal(entries...)}
}

// opsGroup is the and-group of entries.
func opsGroup(entries ...any) conditiontree.Group {
	return groupOf(conditiontree.And, false, entries...)
}

// segmentOf is a Segment's Condition: the and-group of entries.
func segmentOf(entries ...any) conditiontree.Segment {
	return conditiontree.Segment{Group: opsGroup(entries...)}
}

// ruleOf is a Rule's Condition: the and-group of entries.
func ruleOf(entries ...any) conditiontree.Rule {
	return conditiontree.Rule{Group: opsGroup(entries...)}
}

func attribute(name string, test leaf) leaf {
	return leaf{"kind": "attribute", "attribute": name, "test": test}
}

func birthday(test leaf) leaf { return leaf{"kind": "birthday", "test": test} }

func customField(key string, test leaf) leaf {
	return leaf{"kind": "customField", "key": key, "test": test}
}

// property is a test of the triggering Event's property key.
func property(key string, test leaf) leaf {
	return leaf{"kind": "property", "of": leaf{"kind": "trigger"}, "key": key, "test": test}
}

func tagLeaf(name, op string) leaf { return leaf{"kind": "tag", "name": name, "operator": op} }

func telegram(op string) leaf { return leaf{"kind": "telegram", "operator": op} }

func sms(op string) leaf { return leaf{"kind": "sms", "operator": op} }

// eventLeaf is an event count leaf; withinDays 0 leaves the window out (all time).
func eventLeaf(action, op string, times, withinDays int) leaf {
	l := leaf{"kind": "event", "action": action, "operator": op, "times": times}
	if withinDays != 0 {
		l["withinDays"] = withinDays
	}
	return l
}

func text(op, value string) leaf { return leaf{"kind": "text", "operator": op, "value": value} }

func number(op string, value float64) leaf {
	return leaf{"kind": "number", "operator": op, "value": value}
}

func moment(op string, at time.Time) leaf { return leaf{"kind": "moment", "operator": op, "at": at} }

func withinLast(days int) leaf { return leaf{"kind": "withinLast", "days": days} }

func presence(op string) leaf { return leaf{"kind": "presence", "operator": op} }
