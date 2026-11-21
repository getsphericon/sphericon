package condition_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// engagements are the counts that make a Condition able to be unknown (ADR 0181): an open or a
// click, whose tracking a Contact may have refused.
var engagements = []string{systemevent.EmailOpened, systemevent.EmailClicked, systemevent.SmsClicked, systemevent.TelegramClicked}

// negated is the group of entries, negated.
func negated(entries ...any) conditiontree.Group {
	return groupOf(conditiontree.And, true, entries...)
}

func TestAConditionCanBeUnknownWhenItCountsAnEngagement(t *testing.T) {
	for _, action := range engagements {
		count := eventLeaf(action, "atLeast", 1, 0)
		within := eventLeaf(action, "atMost", 2, 30)

		counted := condition.CanBeUnknown(opsGroup(count))
		countedWithin := condition.CanBeUnknown(opsGroup(within))
		nested := condition.CanBeUnknown(opsGroup(attribute("email", text("eq", "a@example.com")), opsGroup(opsGroup(count))))
		underNot := condition.CanBeUnknown(opsGroup(negated(count)))

		assert.True(t, counted, "%s counted", action)
		assert.True(t, countedWithin, "%s counted within a window", action)
		assert.True(t, nested, "%s nested", action)
		assert.True(t, underNot, "%s under a negation", action)
	}

	for name, g := range map[string]conditiontree.Group{
		"the empty Condition":                                    conditiontree.Empty(),
		"a count of another kind":                                opsGroup(eventLeaf("signup", "atLeast", 1, 0), negated(eventLeaf(systemevent.EmailSent, "atLeast", 1, 0))),
		"tests on the Contact":                                   opsGroup(attribute("email", text("eq", "a@example.com")), tagLeaf("vip", "has"), opsGroup(telegram("has"))),
		"a triggering Event's property named like an engagement": opsGroup(property(systemevent.EmailOpened, text("eq", "x"))),
	} {
		unknown := condition.CanBeUnknown(g)

		assert.False(t, unknown, name)
	}
}

// Whatever the tree, a Condition can be unknown exactly when one of its leaves, at any depth and
// under any negation, counts an engagement.
func TestCanBeUnknownAgreesWithItsLeavesAtAnyDepth(t *testing.T) {
	actions := append([]string{"signup", systemevent.EmailSent}, engagements...)
	rapid.Check(t, func(rt *rapid.T) {
		var counted []string
		var tree func(depth int) conditiontree.Group
		tree = func(depth int) conditiontree.Group {
			n := rapid.IntRange(0, 3).Draw(rt, "entries")
			entries := make([]any, 0, n)
			for range n {
				switch {
				case depth < 3 && rapid.Bool().Draw(rt, "nest"):
					entries = append(entries, tree(depth+1))
				case rapid.Bool().Draw(rt, "count"):
					action := rapid.SampledFrom(actions).Draw(rt, "action")
					counted = append(counted, action)
					entries = append(entries, eventLeaf(action, "atLeast", 1, 0))
				default:
					entries = append(entries, tagLeaf("vip", "has"))
				}
			}
			return groupOf(conditiontree.And, rapid.Bool().Draw(rt, "not"), entries...)
		}
		g := tree(0)
		want := slices.ContainsFunc(counted, func(a string) bool { return slices.Contains(engagements, a) })

		got := condition.CanBeUnknown(g)

		assert.Equal(rt, want, got, "counts %v", counted)
	})
}
