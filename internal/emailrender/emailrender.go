// Package emailrender renders broadcast/template email content. Bodies are
// authored as MJML; the pipeline is: Liquid merge tags → MJML compile (which
// emits email-safe, inlined HTML) → derived plain-text part. Each step is a
// maintained library (osteele/liquid, preslavrachev/gomjml, k3a/html2text) — we
// don't hand-roll templating, MJML, or HTML stripping.
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
	"github.com/preslavrachev/gomjml/mjml"
)

// engine is safe for concurrent ParseAndRender calls.
var engine = newEngine()

// errFileTags is what {% include %} and {% render %} fail with: template authors
// are workspace members and API clients, never trusted with the server's files.
var errFileTags = errors.New("include and render tags are not supported")

// noFiles is the template store behind those tags. The library default reads
// from the process working directory, so an author could print any file the
// server can read (/proc/self/environ among them).
type noFiles struct{}

func (noFiles) ReadTemplate(string) ([]byte, error) { return nil, errFileTags }

func newEngine() *liquid.Engine {
	e := liquid.NewEngine()
	e.RegisterTemplateStore(noFiles{})
	return e
}

// Email is a fully-rendered message ready for tracking + delivery.
type Email struct {
	Subject string
	HTML    string
	Text    string
}

// RenderEmail renders subject + MJML body for one recipient: Liquid merge tags
// first (so a {% if %} can change MJML structure per recipient), then MJML
// compiles to email HTML, then the text part is derived from that HTML (before
// any tracking is layered on). A non-nil error means the Liquid merge tags or the
// MJML failed to render — the caller must not send that message (sending the raw
// template to a recipient is worse than not sending).
func RenderEmail(subject, body string, bindings map[string]any) (Email, error) {
	renderedSubject, err := Render(subject, bindings)
	if err != nil {
		return Email{}, fmt.Errorf("render subject: %w", err)
	}
	renderedBody, err := Render(body, bindings)
	if err != nil {
		return Email{}, fmt.Errorf("render body: %w", err)
	}

	html, err := mjml.Render(renderedBody)
	if err != nil {
		return Email{}, fmt.Errorf("compile mjml: %w", err)
	}

	return Email{Subject: renderedSubject, HTML: html, Text: HTMLToText(html)}, nil
}

// Validate reports whether subject and body can render at all, without any recipient
// data: Liquid syntax and MJML compilation errors surface; unbound merge tags simply
// render empty. Planning code calls it so a template that would fail every recipient
// fails the whole send once, up front (ADR 0015).
func Validate(subject, body string) error {
	_, err := RenderEmail(subject, body, map[string]any{})
	return err
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
