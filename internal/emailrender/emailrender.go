// Package emailrender renders broadcast/template email content. Bodies are
// email-ready HTML with Liquid merge tags (ADR 0040); the pipeline is: Liquid
// merge tags → derived plain-text part. Each step is a maintained library
// (osteele/liquid, k3a/html2text, x/net/html) — we don't hand-roll templating or
// HTML stripping.
//
// Tracking (open pixel, click rewriting, unsubscribe footer) is deliberately NOT
// done here: the caller layers it on AFTER this pipeline so re-parsing can never
// mangle the injected pixel or escape the click-URL query strings.
package emailrender

import (
	"errors"
	"fmt"

	"github.com/k3a/html2text"
	"github.com/osteele/liquid"
	"github.com/osteele/liquid/render"
)

// ErrInvalidContent is what Validate fails with: the subject or body cannot render, so
// it is refused where it is saved and where a send is planned. The wrapped text is the
// reason shown to the author.
var ErrInvalidContent = errors.New("message content cannot be rendered")

// engine renders the subject and any other plain-text template: no escaping. bodyEngine
// renders the HTML body: the value of every {{ }} is HTML-escaped as it is written, and
// {{ value | safe }} is the explicit raw output (for a Transactional variable that carries
// HTML on purpose). Both are safe for concurrent ParseAndRender calls.
var (
	engine     = newEngine(false)
	bodyEngine = newEngine(true)
)

// errFileTags is what {% include %} and {% render %} fail with: template authors
// are workspace members and API clients, never trusted with the server's files.
var errFileTags = errors.New("include and render tags are not supported")

// noFiles is the template store behind those tags. The library default reads
// from the process working directory, so an author could print any file the
// server can read (/proc/self/environ among them).
type noFiles struct{}

func (noFiles) ReadTemplate(string) ([]byte, error) { return nil, errFileTags }

func newEngine(escapeHTML bool) *liquid.Engine {
	e := liquid.NewEngine()
	e.RegisterTemplateStore(noFiles{})
	if escapeHTML {
		e.SetAutoEscapeReplacer(render.HtmlEscaper)
	}
	return e
}

// Email is a fully-rendered message ready for tracking + delivery.
type Email struct {
	Subject string
	HTML    string
	Text    string
}

// RenderEmail renders subject + HTML body for one recipient: Liquid merge tags
// first (so a {% if %} can change the markup per recipient; the subject is plain text
// and unescaped, body values are HTML-escaped), then the whole rendered body is
// sanitized again (a variable can carry markup or a URL the save-time pass never saw),
// then the text part is derived from that HTML (before any tracking is layered on). A non-nil error means
// the Liquid merge tags failed to render — the caller must not send that message
// (sending the raw template to a recipient is worse than not sending).
func RenderEmail(subject, body string, bindings map[string]any) (Email, error) {
	renderedSubject, rendered, err := renderBoth(subject, body, bindings)
	if err != nil {
		return Email{}, err
	}
	html := sanitizeRendered(rendered)
	return Email{Subject: renderedSubject, HTML: html, Text: HTMLToText(html)}, nil
}

// renderBoth runs Liquid over the subject (plain text) and the body (HTML-escaped values),
// before any sanitizing.
func renderBoth(subject, body string, bindings map[string]any) (string, string, error) {
	renderedSubject, err := Render(subject, bindings)
	if err != nil {
		return "", "", fmt.Errorf("render subject: %w", err)
	}
	rendered, err := bodyEngine.ParseAndRenderString(body, bindings)
	if err != nil {
		return "", "", fmt.Errorf("render body: %w", err)
	}
	return renderedSubject, rendered, nil
}

// Validate reports whether subject and body can render at all, without any recipient
// data: a Liquid syntax error or HTML that does not parse is refused with the reason
// (ErrInvalidContent); unbound merge tags simply render empty. Save paths and send
// planning call it so a message that would fail every recipient fails once, up front
// (ADR 0015, ADR 0040). Nothing else blocks. The parse check runs on the body as the
// author wrote it: sanitizing would repair what it should refuse.
func Validate(subject, body string) error {
	_, rendered, err := renderBoth(subject, body, map[string]any{})
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidContent, err)
	}
	if err := checkHTML(rendered); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidContent, err)
	}
	return nil
}

// Render renders a Liquid template with the given bindings. On a template error
// it returns the original template unchanged together with the error (useful for
// previews); RenderEmail treats that error as fatal.
func Render(tmpl string, bindings map[string]any) (string, error) {
	out, err := engine.ParseAndRenderString(tmpl, bindings)
	if err != nil {
		return tmpl, err
	}
	return out, nil
}

// HTMLToText derives a plain-text representation of an HTML body for the
// text/plain MIME part.
func HTMLToText(html string) string {
	return html2text.HTML2Text(html)
}

// PrepareFields is what every save path calls (site API, external API and MCP all reach
// it through the broadcasts and templates modules): it refuses content that cannot render
// (Validate) and returns the body to store, sanitized (SanitizeHTML). A nil field is not
// provided, so it is not checked; a nil body is returned as nil.
func PrepareFields(subject, body *string) (*string, error) {
	var s, b string
	if subject != nil {
		s = *subject
	}
	if body != nil {
		b = *body
	}
	if err := Validate(s, b); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}
	clean := SanitizeHTML(b)
	return &clean, nil
}
