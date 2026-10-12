package emailrender_test

import (
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A representative MJML template exercising the common components.
const sampleMJML = `<mjml>
  <mj-head><mj-style>.brand { color: #ff0000; }</mj-style></mj-head>
  <mj-body>
    <mj-section>
      <mj-column>
        <mj-text>Hello {{ first_name }}, welcome!</mj-text>
        <mj-button href="https://example.com/sale">Shop now</mj-button>
      </mj-column>
    </mj-section>
  </mj-body>
</mjml>`

func TestRenderEmailMJML(t *testing.T) {
	out, err := emailrender.RenderEmail("Hi {{ first_name }}", sampleMJML, map[string]any{
		"first_name": "Alex",
	})
	require.NoError(t, err)

	assert.Equal(t, "Hi Alex", out.Subject)
	// Merge tag resolved and MJML compiled to table-based email HTML.
	assert.Contains(t, out.HTML, "Alex")
	assert.NotContains(t, out.HTML, "{{")
	assert.Contains(t, strings.ToLower(out.HTML), "<table")
	assert.Contains(t, out.HTML, "https://example.com/sale")
	assert.NotEmpty(t, out.Text)
}

func TestRenderEmailInvalidMJMLFails(t *testing.T) {
	_, err := emailrender.RenderEmail("s", "<mjml><not-valid", nil)
	assert.Error(t, err)
}

func TestValidateCatchesBrokenTemplatesWithoutRecipientData(t *testing.T) {
	require.NoError(t, emailrender.Validate("Hi {{ first_name }}", sampleMJML), "unbound merge tags are fine")
	require.Error(t, emailrender.Validate("{% if %}", sampleMJML), "liquid syntax error in the subject")
	require.Error(t, emailrender.Validate("s", "<mjml><not-valid"), "mjml that cannot compile")
}
