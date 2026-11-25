package conditiontree_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// A Condition's size counts the tests of an event leaf's filter too: a group, an event leaf, and
// its filter's group with two tests are five nodes.
func TestSizeCountsAnEventLeavesFilter(t *testing.T) {
	var g conditiontree.Group
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","entries":[
		{"kind":"event","action":"order.completed","operator":"atLeast","times":1,"filter":{"kind":"group","combinator":"or","entries":[
			{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}},
			{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"refunded"}}
		]}}
	]}`), &g))

	size := g.Size()

	assert.Equal(t, 5, size)
}
