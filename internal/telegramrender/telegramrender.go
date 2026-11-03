// Package telegramrender renders the text of a Telegram message (ADR 0078): Liquid merge tags,
// whose values are HTML-escaped, then the whole result sanitized to the subset of HTML Telegram
// accepts and balanced, so what leaves sphericon always parses on Telegram's side. Tracked links
// and buttons are layered on by the Outbound send, not here.
package telegramrender

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

// MaxText is the longest text Telegram accepts in a message, in characters.
const MaxText = 4096

// MaxCaption is the longest caption Telegram accepts under a photo, in characters.
const MaxCaption = 1024

// MaxButtons is the most link buttons a message carries, one per row.
// contract-copy: the contract caps `buttons` at 8 too (); the domain refuses it for callers that skip the API
const MaxButtons = 8

// MaxUnsubscribeButton is the longest label of an unsubscribe button, in characters.
// contract-copy: the contract caps `unsubscribeButton` at 64 too; the domain refuses it for callers that skip the API
const MaxUnsubscribeButton = 64

// Limit is the longest text a message may carry: a caption when it has an image, else a text.
func Limit(hasImage bool) int {
	if hasImage {
		return MaxCaption
	}
	return MaxText
}

// TooLongError is a text that, as written, is over the limit of its message: Telegram refuses it.
type TooLongError struct {
	// Length is the text's length as written, in UTF-16 code units; Limit is what the message takes.
	Length, Limit int
}

func (e *TooLongError) Error() string {
	return fmt.Sprintf("the text is %d characters long, Telegram takes %d", e.Length, e.Limit)
}

// CheckWritten is the written-length rule of a message, the one an Automation's Publish and a
// Broadcast's Schedule share: the text as written (merge tags unrendered) may be Limit(hasImage)
// long, in the units an editor counts. What it renders to may still be longer or shorter; a message
// over the limit once rendered fails its own send. The error is a *TooLongError.
func CheckWritten(text string, hasImage bool) error {
	if n, limit := WrittenLength(text), Limit(hasImage); n > limit {
		return &TooLongError{Length: n, Limit: limit}
	}
	return nil
}

// Button is a link button under a message: a label and the URL it opens, shown as an inline URL
// button on a row of its own.
type Button struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ErrContent is a Telegram message's image or buttons that Telegram would refuse.
var ErrContent = errors.New("telegram content")

// CheckImage refuses an image URL that is not an absolute https URL. Blank means no image.
func CheckImage(imageURL string) error {
	if strings.TrimSpace(imageURL) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(imageURL))
	if err != nil || u.Host == "" || !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("%w: the image must be an absolute https URL", ErrContent)
	}
	return nil
}

// CheckUnsubscribeButton refuses a label of an unsubscribe button longer than MaxUnsubscribeButton
// characters. Blank means no button.
func CheckUnsubscribeButton(label string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(label)); n > MaxUnsubscribeButton {
		return fmt.Errorf("%w: the unsubscribe button's label is %d characters long, up to %d", ErrContent, n, MaxUnsubscribeButton)
	}
	return nil
}

// CheckButtons refuses more than MaxButtons buttons, a button with no label and one whose URL is
// not an absolute http or https URL.
func CheckButtons(buttons []Button) error {
	if len(buttons) > MaxButtons {
		return fmt.Errorf("%w: at most %d buttons, got %d", ErrContent, MaxButtons, len(buttons))
	}
	for i, b := range buttons {
		if strings.TrimSpace(b.Label) == "" {
			return fmt.Errorf("%w: button %d needs a label", ErrContent, i+1)
		}
		u, err := url.Parse(strings.TrimSpace(b.URL))
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("%w: button %d needs an absolute http or https URL", ErrContent, i+1)
		}
	}
	return nil
}

// allowed are the tags Telegram's HTML subset keeps; every other tag is dropped and its text kept.
var allowed = map[string]bool{
	"b": true, "strong": true, "i": true, "em": true, "u": true, "ins": true,
	"s": true, "strike": true, "del": true, "a": true, "code": true, "pre": true,
	"tg-spoiler": true, "blockquote": true,
}

// dropped are the tags whose content is no text of the message.
var dropped = map[string]bool{"script": true, "style": true, "template": true, "head": true, "title": true}

// Render renders text for one recipient: Liquid first (a value is HTML-escaped as it is written, so
// a merge tag never adds markup), then the sanitizer. An error is a Liquid error: the message must
// not be sent as written.
func Render(text string, bindings map[string]any) (string, error) {
	rendered, err := emailrender.RenderBody(text, bindings)
	if err != nil {
		return "", fmt.Errorf("render text: %w", err)
	}
	return strings.TrimSpace(Sanitize(rendered)), nil
}

// Check refuses a text whose Liquid does not render, with no recipient data (unbound merge tags
// render empty): the check of a save.
func Check(text string) error {
	_, err := Render(text, map[string]any{})
	return err
}

// Sanitize reduces markup to Telegram's subset: the allowed tags, `a` only with an http or https
// href, text escaped, tags balanced (the HTML parser closes what is left open and repairs
// misnested ones). Inside code and pre nothing but text is kept, as Telegram nests nothing there.
func Sanitize(markup string) string {
	nodes, err := html.ParseFragment(strings.NewReader(markup), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return escapeText(markup)
	}
	var b strings.Builder
	for _, n := range nodes {
		write(&b, n, false)
	}
	return b.String()
}

func write(b *strings.Builder, n *html.Node, flat bool) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(escapeText(n.Data))
	case html.ElementNode:
		element(b, n, flat)
	case html.ErrorNode, html.DocumentNode, html.CommentNode, html.DoctypeNode, html.RawNode:
	}
}

func element(b *strings.Builder, n *html.Node, flat bool) {
	switch {
	case dropped[n.Data]:
		return
	case n.Data == "br":
		b.WriteString("\n")
		return
	case !allowed[n.Data] || flat:
		children(b, n, flat)
		return
	}
	opening, closing := "<"+n.Data+">", "</"+n.Data+">"
	if n.Data == "a" {
		href, ok := safeHref(n)
		if !ok {
			children(b, n, flat)
			return
		}
		opening = `<a href="` + escapeAttr(href) + `">`
	}
	var inner strings.Builder
	children(&inner, n, n.Data == "code" || n.Data == "pre")
	b.WriteString(opening)
	if n.Data == "pre" && strings.HasPrefix(inner.String(), "\n") {
		// The HTML parser drops a newline right after <pre>; keep the one the text begins with.
		b.WriteString("\n")
	}
	b.WriteString(inner.String())
	b.WriteString(closing)
}

func children(b *strings.Builder, n *html.Node, flat bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		write(b, c, flat)
	}
}

// safeHref is the href of an `a` when it is an absolute http or https URL.
func safeHref(n *html.Node) (string, bool) {
	for _, a := range n.Attr {
		if a.Key != "href" {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(a.Val))
		if err != nil || u.Host == "" {
			return "", false
		}
		if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
			return "", false
		}
		return u.String(), true
	}
	return "", false
}

var (
	textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	attrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
)

func escapeText(s string) string { return textEscaper.Replace(s) }
func escapeAttr(s string) string { return attrEscaper.Replace(s) }

// VisibleLength is the length of the text of markup as Telegram counts it against the limit: the
// text after tags are removed and entities read, in UTF-16 code units.
func VisibleLength(markup string) int {
	z := html.NewTokenizer(strings.NewReader(markup))
	n := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return n
		case html.TextToken:
			n += WrittenLength(html.UnescapeString(string(z.Text())))
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken, html.CommentToken, html.DoctypeToken:
		}
	}
}

// WrittenLength is the length of text as written, in UTF-16 code units, the way an editor counts
// it. A text longer than MaxText is refused where a send is armed.
func WrittenLength(text string) int { return len(utf16.Encode([]rune(text))) }
