package emailrender_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// A variable's value is HTML-escaped where the body writes it: a Contact's data cannot add markup.
func TestRenderEmailEscapesVariablesInTheBody(t *testing.T) {
	out, err := emailrender.RenderEmail("Hi {{ first_name }}", `<p>Hello {{ first_name }}</p>`,
		map[string]any{"first_name": `<img src=x onerror=alert(1)> & "Co"`})
	require.NoError(t, err)

	assert.Contains(t, out.HTML, `<p>Hello &lt;img src=x onerror=alert(1)&gt; &amp; &#34;Co&#34;</p>`, "markup is shown as text")
	assert.NotContains(t, out.HTML, "<img")
	assert.Equal(t, `Hi <img src=x onerror=alert(1)> & "Co"`, out.Subject, "the subject is plain text with no entities")
}

// {{ value | safe }} writes a variable's markup as it is: the explicit raw output (ADR 0118).
func TestRenderEmailWritesSafeOutputAsItIs(t *testing.T) {
	vars := map[string]any{"block": `<b>bold</b><span style="color:red">red</span>`}

	escaped, err := emailrender.RenderEmail("s", `<p>{{ block }}</p>`, vars)
	require.NoError(t, err)
	raw, err := emailrender.RenderEmail("s", `<p>{{ block | safe }}</p>`, vars)
	require.NoError(t, err)

	assert.Contains(t, escaped.HTML, "&lt;b&gt;bold&lt;/b&gt;")
	assert.Contains(t, raw.HTML, `<p><b>bold</b><span style="color:red">red</span></p>`)
}

func TestRenderEmailKeepsLiquidControlFlowAroundTableRows(t *testing.T) {
	body := `<table><tbody>{% for i in items %}<tr><td>{{ i }}</td></tr>{% endfor %}</tbody></table>`
	out, err := emailrender.RenderEmail("s", body, map[string]any{"items": []string{"a", "b"}})
	require.NoError(t, err)
	assert.Contains(t, out.HTML, "<tr><td>a</td></tr><tr><td>b</td></tr>")
}

// The message is sent as the body renders: nothing the author wrote is dropped on the way.
func TestRenderEmailSendsTheBodyAsWritten(t *testing.T) {
	body := `<!--[if mso]><table><tr><td><![endif]--><p class="a" onclick="track()">Hi {{ name }}</p>` +
		`<style>@keyframes pulse{from{opacity:0}}</style><!--[if mso]></td></tr></table><![endif]-->`

	out, err := emailrender.RenderEmail("s", body, map[string]any{"name": "Ann"})
	require.NoError(t, err)

	assert.Equal(t, `<!--[if mso]><table><tr><td><![endif]--><p class="a" onclick="track()">Hi Ann</p>`+
		`<style>@keyframes pulse{from{opacity:0}}</style><!--[if mso]></td></tr></table><![endif]-->`, out.HTML)
}

// A body that renders passes as it was written, whatever a mail client may drop of it (ADR 0118).
func TestCheckFieldsTakesABodyThatRenders(t *testing.T) {
	subject, body := "Hi {{ first_name }}", `<p onclick="track()">Hello {{ first_name }}</p><form action="https://example.com/s"></form>`

	err := emailrender.CheckFields(&subject, &body)

	require.NoError(t, err)
}

func TestCheckFieldsRefusesWhatCannotRender(t *testing.T) {
	broken := `<p>hi</p><div class="a"`

	err := emailrender.CheckFields(nil, &broken)

	require.ErrorIs(t, err, emailrender.ErrInvalidContent)
}
