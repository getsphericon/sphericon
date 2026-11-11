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
	"slices"
	"strings"

	"github.com/k3a/html2text"
	"github.com/osteele/liquid"
	"github.com/osteele/liquid/render"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ErrInvalidContent is what Lint fails with: the subject or body cannot render, so
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
	clean := sanitizeRendered(rendered)
	return Email{Subject: renderedSubject, HTML: clean, Text: HTMLToText(clean)}, nil
}

// renderBoth runs Liquid over the subject (plain text) and the body (HTML-escaped values),
// before any sanitizing.
func renderBoth(subject, body string, bindings map[string]any) (renderedSubject, renderedBody string, err error) {
	renderedSubject, err = Render(subject, bindings)
	if err != nil {
		return "", "", fmt.Errorf("render subject: %w", err)
	}
	rendered, err := bodyEngine.ParseAndRenderString(body, bindings)
	if err != nil {
		return "", "", fmt.Errorf("render body: %w", err)
	}
	return renderedSubject, rendered, nil
}

// RenderBody renders a Liquid template as markup: the value of every {{ }} is HTML-escaped as it
// is written, as in an email body. Another Channel's markup text (Telegram's) renders through it.
func RenderBody(tmpl string, bindings map[string]any) (string, error) {
	return bodyEngine.ParseAndRenderString(tmpl, bindings)
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
func HTMLToText(markup string) string {
	return toText(markup, false)
}

// FinalHTMLToText derives the text/plain part of a send from its final per-recipient HTML
// (ADR 0056): tracking and the unsubscribe footer are already in it, so a link reads as its
// text followed by the URL it carries and the footer's unsubscribe link and postal address
// appear as they do in the HTML. The style, script and head content is left out, and so is any
// element whose inline style hides it (the open pixel, a preheader; see hidingDeclarations).
func FinalHTMLToText(finalHTML string) string {
	return toText(finalHTML, true)
}

// toText is the derivation both text parts share; final drops the hidden elements and writes a
// link as its text and URL. html2text separates words only where the markup has whitespace or a
// paragraph, heading, list or line break, so the blocks of a body are first set apart in the
// parsed tree (separateBlocks): the editor's export has no whitespace between them. The spaces
// that leaves at the ends of a line are trimmed.
func toText(markup string, final bool) string {
	lines := strings.Split(rawText(markup, final), "\r\n")
	for i, line := range lines {
		lines[i] = strings.Trim(line, " ")
	}
	return strings.Join(lines, "\r\n")
}

// rawText is html2text's reading of the markup once its tree is prepared; markup that does not
// parse is read as it is.
func rawText(markup string, final bool) string {
	var opts []html2text.Option
	if final {
		opts = append(opts, html2text.WithLinksInnerText())
	}
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return html2text.HTML2TextWithOptions(markup, opts...)
	}
	if final {
		dropHidden(doc)
	}
	separateBlocks(doc)
	var sb strings.Builder
	if err := html.Render(&sb, doc); err != nil {
		return html2text.HTML2TextWithOptions(markup, opts...)
	}
	return html2text.HTML2TextWithOptions(sb.String(), opts...)
}

// blockElements are the elements that stand apart from the text around them: their content is
// never read as one word with their neighbour's.
var blockElements = map[atom.Atom]bool{
	atom.Address: true, atom.Article: true, atom.Aside: true, atom.Blockquote: true,
	atom.Caption: true, atom.Center: true, atom.Dd: true, atom.Div: true, atom.Dl: true,
	atom.Dt: true, atom.Figcaption: true, atom.Figure: true, atom.Footer: true, atom.H1: true,
	atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true, atom.Header: true,
	atom.Hr: true, atom.Li: true, atom.Main: true, atom.Nav: true, atom.Ol: true, atom.P: true,
	atom.Pre: true, atom.Section: true, atom.Table: true, atom.Tbody: true, atom.Td: true,
	atom.Tfoot: true, atom.Th: true, atom.Thead: true, atom.Tr: true, atom.Ul: true,
}

// separateBlocks puts a space, in place, before and after every block element, as the
// indentation of formatted markup would.
func separateBlocks(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && blockElements[c.DataAtom] {
			n.InsertBefore(&html.Node{Type: html.TextNode, Data: " "}, c)
			n.InsertBefore(&html.Node{Type: html.TextNode, Data: " "}, c.NextSibling)
			separateBlocks(c)
			c = c.NextSibling
			continue
		}
		separateBlocks(c)
	}
}

// hidingDeclarations are the inline style declarations (property, value) mail authors use to
// hide an element from the reader while keeping it in the HTML. Only inline styles are read:
// hiding through a class or a <style> rule is not detected. A zero font size is not one of
// them: it is how a layout table removes the gaps between its columns, and the text inside
// sets its own size.
var hidingDeclarations = map[string][]string{
	"display":    {"none"},
	"visibility": {"hidden"},
	"mso-hide":   {"all"},
	"opacity":    {"0"},
}

// zeroLengths are the values of a zero max-height.
var zeroLengths = []string{"0", "0px", "0pt", "0em"}

// hidesElement reports whether an inline style hides its element: one of the hiding
// declarations, or a zero max-height that clips its content (overflow:hidden), the shape of a
// preheader. A zero max-height alone clips nothing. The values are compared lower-case.
func hidesElement(style string) bool {
	parsed := parseStyle(style)
	for prop, hiding := range hidingDeclarations {
		value := strings.ToLower(parsed[prop])
		if slices.Contains(hiding, value) {
			return true
		}
	}
	maxHeight, overflow := strings.ToLower(parsed["max-height"]), strings.ToLower(parsed["overflow"])
	return slices.Contains(zeroLengths, maxHeight) && overflow == "hidden"
}

// dropHidden removes, in place, every element whose inline style hides it.
func dropHidden(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && slices.ContainsFunc(c.Attr, func(a html.Attribute) bool {
			return a.Key == "style" && hidesElement(a.Val)
		}) {
			n.RemoveChild(c)
		} else {
			dropHidden(c)
		}
		c = next
	}
}

// PrepareFields is what every save path calls (site API, external API and MCP all reach
// it through the broadcasts and templates modules): it refuses content that cannot render
// (Lint) and returns the body to store, sanitized (SanitizeHTML). A nil field is not
// provided, so it is not checked; a nil body is returned as nil.
func PrepareFields(subject, body *string) (*string, error) {
	var s, b string
	if subject != nil {
		s = *subject
	}
	if body != nil {
		b = *body
	}
	if err := Lint(s, b, nil).Error; err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}
	clean := SanitizeHTML(b)
	return &clean, nil
}
