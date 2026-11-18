package condition_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A Condition can be unknown when it counts an open or a click, whose tracking a Contact may have
// refused (ADR 0181): the count of such an Event is not known to be what it says.
func TestAConditionCanBeUnknownWhenItCountsAnEngagement(t *testing.T) {
	negated := func(entries ...any) conditiontree.Group {
		g := opsGroup(entries...)
		g.Not = true
		return g
	}

	for _, action := range []string{systemevent.EmailOpened, systemevent.EmailClicked, systemevent.SmsClicked, systemevent.TelegramClicked} {
		count := eventLeaf(action, "atLeast", 1, 0)
		within := eventLeaf(action, "atMost", 2, 30)

		assert.True(t, condition.CanBeUnknown(opsGroup(count)), "%s counted", action)
		assert.True(t, condition.CanBeUnknown(opsGroup(within)), "%s counted within a window", action)
		assert.True(t, condition.CanBeUnknown(opsGroup(rule("email", "eq", "a@example.com"), opsGroup(opsGroup(count)))), "%s nested", action)
		assert.True(t, condition.CanBeUnknown(opsGroup(negated(count))), "%s under a negation", action)
	}

	for name, g := range map[string]conditiontree.Group{
		"the empty Condition":                                    conditiontree.Empty(),
		"a count of another kind":                                opsGroup(eventLeaf("signup", "atLeast", 1, 0), negated(eventLeaf(systemevent.EmailSent, "atLeast", 1, 0))),
		"tests on the Contact":                                   opsGroup(rule("email", "eq", "a@example.com"), rule("tag", "has", "vip"), opsGroup(rule("telegram", "has", ""))),
		"a triggering Event's property named like an engagement": opsGroup(rule("trigger:"+systemevent.EmailOpened, "eq", "x")),
	} {
		assert.False(t, condition.CanBeUnknown(g), name)
	}
}
