package outboundmessages_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
)

// A message about an Event binds it as `event` (#752): a property renders by its name, also one
// named in brackets, a missing one renders empty, and a value is HTML-escaped like a Contact's.
func TestAnEventsPropertiesRenderByNameAndAMissingOneRendersEmpty(t *testing.T) {
	vars := outboundmessages.EventVariables("payment_failed", map[string]any{"amount": "€49", "plan name": "<b>Pro</b>"})

	email, err := emailrender.RenderEmail("{{ event.action }}",
		`<p>{{ event.properties.amount }}|{{ event.properties["plan name"] }}|[{{ event.properties.missing }}]</p>`,
		outboundmessages.Bindings(&ent.Contact{}, vars))

	require.NoError(t, err)
	assert.Equal(t, "payment_failed", email.Subject)
	assert.Contains(t, email.HTML, "€49|&lt;b&gt;Pro&lt;/b&gt;|[]")
}

// An Enrollment no Event began binds an empty `event`: every tag renders empty, and a Contact's
// custom field named `event` does not stand in for it.
func TestAnEmptyEventRendersEveryTagEmpty(t *testing.T) {
	c := &ent.Contact{CustomFields: map[string]any{"event": map[string]any{"action": "forged"}}}

	email, err := emailrender.RenderEmail("[{{ event.action }}]", "<p>[{{ event.properties.amount }}]</p>",
		outboundmessages.Bindings(c, outboundmessages.EnrollmentVariables(nil, nil)))

	require.NoError(t, err)
	assert.Equal(t, "[]", email.Subject)
	assert.Contains(t, email.HTML, "<p>[]</p>")
}

// An Enrollment binds the Events its Wait for event Steps arrived on as `awaited`, by each Step's
// name (ADR 0136): a name with no copy renders empty, and a Contact's custom field named `awaited`
// stands in for neither.
func TestAwaitedEventsRenderByNameAndACustomFieldNeverStandsIn(t *testing.T) {
	c := &ent.Contact{CustomFields: map[string]any{"awaited": map[string]any{"purchase": map[string]any{"action": "forged", "properties": map[string]any{"product": "forged"}}}}}
	body := "<p>[{{ awaited.purchase.action }}|{{ awaited.purchase.properties.product }}]</p>"
	for name, tc := range map[string]struct {
		awaited map[string]outboundmessages.AwaitedEvent
		renders string
	}{
		"arrived":       {map[string]outboundmessages.AwaitedEvent{"purchase": {Action: "order.created", Properties: map[string]any{"product": "Lamp"}}}, "<p>[order.created|Lamp]</p>"},
		"another name":  {map[string]outboundmessages.AwaitedEvent{"booking": {Action: "call.booked", Properties: map[string]any{"product": "Call"}}}, "<p>[|]</p>"},
		"nothing kept":  {nil, "<p>[|]</p>"},
		"no properties": {map[string]outboundmessages.AwaitedEvent{"purchase": {Action: "order.created"}}, "<p>[order.created|]</p>"},
	} {
		t.Run(name, func(t *testing.T) {
			email, err := emailrender.RenderEmail("Thanks", body, outboundmessages.Bindings(c, outboundmessages.EnrollmentVariables(nil, tc.awaited)))

			require.NoError(t, err)
			assert.Contains(t, email.HTML, tc.renders)
		})
	}
}

// A kept System event's properties bind by their public form, as `event`'s do (ADR 0142).
func TestAKeptSystemEventBindsItsKeysByTheirPublicForm(t *testing.T) {
	awaited := map[string]outboundmessages.AwaitedEvent{"click": {Action: systemevent.EmailClicked, Properties: map[string]any{"outboundMessageId": float64(42)}}}

	email, err := emailrender.RenderEmail("Clicked", "<p>[{{ awaited.click.properties.outboundMessageId }}]</p>",
		outboundmessages.Bindings(&ent.Contact{}, outboundmessages.EnrollmentVariables(nil, awaited)))

	require.NoError(t, err)
	assert.Contains(t, email.HTML, "<p>["+publicid.Encode(42)+"]</p>")
}
