package emailrender_test

import (
	"testing"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A representative email body: tables, an inline style, a merge tag and a link.
const sampleHTML = `<table role="presentation" width="100%"><tbody><tr><td style="color:#222">
<p>Hello {{ first_name }}, welcome!</p>
<br>
<a href="https://example.com/sale">Shop now</a>
<img src="https://example.com/logo.png" alt="Logo">
</td></tr></tbody></table>`

func TestRenderEmailHTML(t *testing.T) {
	out, err := emailrender.RenderEmail("Hi {{ first_name }}", sampleHTML, map[string]any{
		"first_name": "Alex",
	})
	require.NoError(t, err)

	assert.Equal(t, "Hi Alex", out.Subject)
	assert.Contains(t, out.HTML, "Hello Alex, welcome!")
	assert.NotContains(t, out.HTML, "{{")
	assert.Contains(t, out.HTML, "https://example.com/sale")
	assert.Contains(t, out.Text, "Hello Alex, welcome!")
}

func TestLintAcceptsRenderableContent(t *testing.T) {
	require.NoError(t, emailrender.Lint("Hi {{ first_name }}", sampleHTML, nil).Error, "unbound merge tags are fine")
	require.NoError(t, emailrender.Lint("s", "", nil).Error, "an empty body parses")
	require.NoError(t, emailrender.Lint("s", "plain <b>text</b> with <!-- a comment --> and &amp; entities", nil).Error)
}

func TestLintRefusesWhatCannotRender(t *testing.T) {
	for name, tc := range map[string]struct{ subject, body, reason string }{
		"liquid error in the subject": {"{% if %}", sampleHTML, "render subject"},
		"liquid error in the body":    {"s", "<p>{% if %}broken</p>", "render body"},
		"tag left open at the end":    {"s", "<p>hi</p><div class=\"a\"", "html"},
		"element never closed":        {"s", "<div><span>hi</div>", "<span> is never closed"},
		"end tag that closes nothing": {"s", "<p>hi</p></div>", "</div> closes nothing"},
		"unclosed container":          {"s", "<table><tr><td>hi</td></tr>", "<table> is never closed"},
	} {
		t.Run(name, func(t *testing.T) {
			err := emailrender.Lint(tc.subject, tc.body, nil).Error
			require.ErrorIs(t, err, emailrender.ErrInvalidContent)
			assert.ErrorContains(t, err, tc.reason)
		})
	}
}
