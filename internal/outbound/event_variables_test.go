package outbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// A message about an Event binds it as `event` (#752): a property renders by its name, also one
// named in brackets, a missing one renders empty, and a value is HTML-escaped like a Contact's.
func TestAnEventsPropertiesRenderByNameAndAMissingOneRendersEmpty(t *testing.T) {
	vars := outbound.EventVariables("payment_failed", map[string]any{"amount": "€49", "plan name": "<b>Pro</b>"})

	email, err := emailrender.RenderEmail("{{ event.action }}",
		`<p>{{ event.properties.amount }}|{{ event.properties["plan name"] }}|[{{ event.properties.missing }}]</p>`,
		outbound.Bindings(&ent.Contact{}, vars))

	require.NoError(t, err)
	assert.Equal(t, "payment_failed", email.Subject)
	assert.Contains(t, email.HTML, "€49|&lt;b&gt;Pro&lt;/b&gt;|[]")
}

// An Enrollment no Event began binds an empty `event`: every tag renders empty, and a Contact's
// custom field named `event` does not stand in for it.
func TestAnEmptyEventRendersEveryTagEmpty(t *testing.T) {
	c := &ent.Contact{CustomFields: map[string]any{"event": map[string]any{"action": "forged"}}}

	email, err := emailrender.RenderEmail("[{{ event.action }}]", "<p>[{{ event.properties.amount }}]</p>",
		outbound.Bindings(c, outbound.EnrollmentVariables(nil)))

	require.NoError(t, err)
	assert.Equal(t, "[]", email.Subject)
	assert.Contains(t, email.HTML, "<p>[]</p>")
}
