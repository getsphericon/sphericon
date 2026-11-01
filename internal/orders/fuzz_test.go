package orders_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/orders"
)

var (
	fuzzActions = []string{orders.ActionCompleted, orders.ActionUpdated, orders.ActionRefunded, orders.ActionCancelled}
	// The default currency is the Order's own, for a change to it: none, or a valid code with two,
	// zero or three decimal places. An Order Completed has none and must name its currency.
	fuzzFallbacks = []*string{nil, new("EUR"), new("JPY"), new("KWD")}
)

// decodeProps reads the properties as the collect API does (numbers kept as json.Number); a body
// that is no JSON object is an Event without properties.
func decodeProps(data []byte) map[string]any {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var props map[string]any
	if dec.Decode(&props) != nil {
		return nil
	}
	return props
}

// An order Event's properties come from a shop over the collect API: whatever they hold, Parse
// either refuses them with a reason for the sender or reads an Order it can store, never a panic.
func FuzzParseReadsAnOrderOrRefusesIt(f *testing.F) {
	f.Add(uint8(0), uint8(1), []byte(`{"order_id":"A-1","total":"12.50","currency":"USD"}`))
	f.Add(uint8(0), uint8(0), []byte(`{"order_id":"A-1","total":12.5}`))
	f.Add(uint8(1), uint8(2), []byte(`{"order_id":"A-1","total":1200}`))
	f.Add(uint8(2), uint8(1), []byte(`{"order_id":"A-1","total":3,"refund_id":17}`))
	f.Add(uint8(2), uint8(3), []byte(`{"order_id":"A-1","total":"1.234","refund_id":"R-1"}`))
	f.Add(uint8(3), uint8(0), []byte(`{"order_id":"A-1"}`))
	f.Add(uint8(0), uint8(1), []byte(`{"order_id":"","total":-1e400,"currency":"usd"}`))

	f.Fuzz(func(t *testing.T, action, fallback uint8, data []byte) {
		act := fuzzActions[int(action)%len(fuzzActions)]
		props := decodeProps(data)

		p, err := orders.Parse(act, props, fuzzFallbacks[int(fallback)%len(fuzzFallbacks)])

		assert.Truef(t, err == nil || orders.IsRefusal(err), "%s %s: an error that is no refusal: %v", act, data, err)
		assert.Truef(t, err != nil || p.OrderID != "", "%s %s: an Order without an id", act, data)
		assert.Truef(t, err != nil || p.Deferred == (p.Currency == ""), "%s %s: deferred %v in currency %q", act, data, p.Deferred, p.Currency)
		assert.Truef(t, err != nil || !p.Deferred || act != orders.ActionCompleted, "%s %s: an Order Completed in no currency", act, data)
		assert.Truef(t, err != nil || p.HasTotal || act == orders.ActionCancelled, "%s %s: no total", act, data)
		assert.Truef(t, err != nil || !p.Deferred || p.Total == 0, "%s %s: a deferred total read as %d", act, data, p.Total)
		assert.Truef(t, err != nil || p.RefundID == "" || act == orders.ActionRefunded, "%s %s: a refund id on another action", act, data)
		assert.Subset(t, []string{"order_id", "total", "currency", "refund_id"}, lo.Keys(orders.Read(props)))
	})
}
