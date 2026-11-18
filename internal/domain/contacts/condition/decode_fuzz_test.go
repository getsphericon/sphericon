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
	f.Add(`{"kind":"rule","field":"email","operator":"eq","value":"x"}`)
	f.Add(`{"kind":"rule","field":"tag","operator":"has","value":"vip"}`)
	f.Add(`{"kind":"rule","field":"custom:plan","operator":"contains","value":"pro"}`)
	f.Add(`{"kind":"rule","field":"trigger:url","operator":"null","value":""}`)
	f.Add(`{"kind":"number","field":"custom:score","operator":"gte","value":9.5}`)
	f.Add(`{"kind":"date","field":"birthday","operator":"before","at":"1990-01-01T00:00:00Z"}`)
	f.Add(`{"kind":"dateWithin","field":"custom:seen_at","withinDays":7}`)
	f.Add(`{"kind":"event","action":"email.opened","operator":"atLeast","times":1,"withinDays":30}`)
	f.Add(`{"kind":"group","combinator":"or","not":true,"rules":[{"kind":"event","action":"sms.clicked","operator":"atMost","times":0}]}`)
	f.Add(`{"kind":"group","combinator":"and","rules":"nope"}`)
	f.Add(`{"kind":"rule","field":"custom:","operator":"eq","value":{"a":1}}`)
	f.Add(`42`)

	f.Fuzz(func(t *testing.T, entry string) {
		g := conditiontree.Group{Kind: conditiontree.KindGroup, Combinator: conditiontree.And, Rules: []json.RawMessage{json.RawMessage(entry)}}
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
