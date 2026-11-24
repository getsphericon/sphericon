package condition_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
)

// A stored Condition is a document a client wrote, read by one decode. Whatever its entries, the
// readers built on that decode never panic, and none of them finds what the document does not
// hold: no engagement count without an engagement's name, no Tag without the Tag's name.
func FuzzDecodeReadsAnyDocument(f *testing.F) {
	f.Add(`{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"eq","value":"x"}}`)
	f.Add(`{"kind":"tag","name":"vip","operator":"has"}`)
	f.Add(`{"kind":"customField","key":"plan","test":{"kind":"text","operator":"contains","value":"pro"}}`)
	f.Add(`{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"presence","operator":"notSet"}}`)
	f.Add(`{"kind":"customField","key":"score","test":{"kind":"number","operator":"gte","value":9.5}}`)
	f.Add(`{"kind":"birthday","test":{"kind":"moment","operator":"before","at":"1990-01-01T00:00:00Z"}}`)
	f.Add(`{"kind":"customField","key":"seen_at","test":{"kind":"withinLast","days":7}}`)
	f.Add(`{"kind":"event","action":"email.opened","operator":"atLeast","times":1,"withinDays":30}`)
	f.Add(`{"kind":"group","combinator":"or","not":true,"entries":[{"kind":"event","action":"sms.clicked","operator":"atMost","times":0}]}`)
	f.Add(`{"kind":"group","combinator":"and","entries":"nope"}`)
	f.Add(`{"kind":"customField","key":"","test":{"kind":"text","operator":"eq","value":{"a":1}}}`)
	f.Add(`{"kind":"telegram","operator":"has"}`)
	f.Add(`{"kind":"sms","operator":"doesNotHave"}`)
	f.Add(`{"kind":"engagementTracking","channel":"telegram","test":{"kind":"text","operator":"neq","value":"denied"}}`)
	f.Add(`42`)

	f.Fuzz(func(t *testing.T, entry string) {
		g := conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{json.RawMessage(entry)}}
		names := strings.Contains(strings.ToLower(entry), "vip")
		counts := strings.Contains(entry, "opened") || strings.Contains(entry, "clicked")

		unknown := condition.CanBeUnknown(g)
		named := condition.NamesTag(g, "vip")
		fits := condition.FitsCustomFieldType(g, "plan", customfield.TypeNumber)

		assert.False(t, unknown && !counts, "%q can be unknown without counting an engagement", entry)
		assert.False(t, named && !names, "%q names a Tag it does not hold", entry)
		assert.True(t, fits || strings.Contains(entry, "plan"), "%q misfits a field it does not test", entry)
	})
}

// A filter is a document a client wrote, compiled to a path Postgres parses. Whatever its entries,
// the compiler gives a path in filter form with its variables as one JSON object, or an error,
// never both.
func FuzzCompileEventFilterReadsAnyDocument(f *testing.F) {
	f.Add(`{"kind":"property","key":"tag","test":{"kind":"text","operator":"eq","value":"vip"}}`)
	f.Add(`{"kind":"property","key":"url","test":{"kind":"text","operator":"contains","value":"\") || (\""}}`)
	f.Add(`{"kind":"property","key":"total","test":{"kind":"number","operator":"gte","value":9.5}}`)
	f.Add(`{"kind":"property","key":"coupon","test":{"kind":"presence","operator":"notSet"}}`)
	f.Add(`{"kind":"group","combinator":"or","not":true,"entries":[{"kind":"property","key":"a","test":{"kind":"text","operator":"beginsWith","value":"x"}}]}`)
	f.Add(`{"kind":"property","of":{"kind":"trigger"},"key":"tag","test":{"kind":"text","operator":"eq","value":"vip"}}`)
	f.Add(`{"kind":"tag","name":"vip","operator":"has"}`)
	f.Add(`{"kind":"group","combinator":"and","entries":"nope"}`)
	f.Add(`42`)

	f.Fuzz(func(t *testing.T, entry string) {
		g := conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Entries: []json.RawMessage{json.RawMessage(entry)}}

		path, err := condition.CompileEventFilter(conditiontree.EventFilter{Group: g})

		compiled := strings.HasPrefix(path.Text, "strict $ ? (") && json.Valid(path.Vars)
		assert.NotEqual(t, err == nil, path.Text == "", "%q: a path or an error, never both nor neither", entry)
		assert.Equal(t, err == nil, compiled, "%q: %q %s", entry, path.Text, path.Vars)
	})
}
