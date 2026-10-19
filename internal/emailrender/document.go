package emailrender

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// Authored is a message written as an editor document: the body the server built from it and
// the editor version that built it. A save stores both beside the document, so a stored
// document always has its HTML and they never disagree (ADR 0040).
type Authored struct {
	// Body is the stored HTML: the editor's export, with its React markers removed and its merge
	// tags unescaped, validated as content and sanitized.
	Body string
	// EditorVersion is the version of the editor whose export code rendered the document.
	EditorVersion string
}

// FromDocument is the one step from an editor document to the body to store, shared by every
// place that holds a `doc` beside a `body` (Templates, Broadcasts, Automation steps). It runs
// the document renderer (the only caller on the write path: the send path never renders a
// document), then makes the export storable: the render's own markers are dropped, the quotes
// the editor HTML-escapes inside {{ }} and {% %} are restored so Liquid can read them, and the
// result goes through PrepareFields like any body, so the stored HTML survives SanitizeHTML
// unchanged.
//
// A document that is invalid or too large for the editor is ErrInvalidContent with the reason;
// subject, when not nil, is checked with the body. A render that could not run (the request
// ended, or a time or memory limit of ours stopped it) says nothing about the document: it is
// ErrRenderUnavailable, which a client may retry. A failure of the runtime itself is returned as
// it is.
func FromDocument(ctx context.Context, subject *string, doc []byte) (Authored, error) {
	out, err := emaildoc.Default().Render(ctx, doc)
	if err != nil {
		switch {
		case isDocumentRefusal(err):
			return Authored{}, fmt.Errorf("%w: %w", ErrInvalidContent, err)
		case isRenderUnavailable(err):
			return Authored{}, fmt.Errorf("%w: %w", ErrRenderUnavailable, err)
		}
		return Authored{}, err
	}
	exported := storable(out.HTML)
	body, err := PrepareFields(subject, &exported)
	if err != nil {
		return Authored{}, err
	}
	return Authored{Body: *body, EditorVersion: emaildoc.EditorVersion()}, nil
}

// ErrRenderUnavailable is a document render that could not run to its end for a reason that is not
// the document's: the request's context ended, or the render hit the time or memory limit of the
// server. It is retryable and never a claim that the document is invalid.
var ErrRenderUnavailable = errors.New("the email could not be rendered right now, try again")

// Content is what a write stores of a message's content.
type Content struct {
	// Body is the body to store; nil when the write names none.
	Body *string
	// Authored is set when the content is a document: its body is Body.
	Authored *Authored
}

// ResolveContent resolves the body of a write: the one place Templates, Broadcasts and Automation
// steps decide it. A document is rendered to it (the one render of the save, never on the send
// path); a body is validated and sanitized; neither leaves the content alone; both is errBoth,
// the caller's own refusal.
func ResolveContent(ctx context.Context, subject, body *string, doc []byte, errBoth error) (Content, error) {
	if len(doc) == 0 {
		b, err := PrepareFields(subject, body)
		return Content{Body: b}, err
	}
	if body != nil {
		return Content{}, errBoth
	}
	authored, err := FromDocument(ctx, subject, doc)
	if err != nil {
		return Content{}, err
	}
	return Content{Body: &authored.Body, Authored: &authored}, nil
}

// isDocumentRefusal tells what the author can fix (the document) from what they cannot.
func isDocumentRefusal(err error) bool {
	for _, target := range []error{
		emaildoc.ErrInvalid, emaildoc.ErrTooLarge, emaildoc.ErrTooDeep, emaildoc.ErrTooManyNodes,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// isRenderUnavailable is a render that ended for a reason outside the document.
func isRenderUnavailable(err error) bool {
	for _, target := range []error{
		emaildoc.ErrTimeout, emaildoc.ErrMemoryLimit, context.Canceled, context.DeadlineExceeded,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

var (
	// renderMarkers are the comments React's server renderer leaves in the export.
	renderMarkers = strings.NewReplacer("<!--$-->", "", "<!--/$-->", "", "<!--html-->", "", "<!--head-->", "", "<!--body-->", "")
	liquidMarkup  = regexp.MustCompile(`(?s)\{\{.*?\}\}|\{%.*?%\}`)
)

// storable turns the editor's export into a body: markers out, Liquid markup unescaped.
func storable(exported string) string {
	return liquidMarkup.ReplaceAllStringFunc(renderMarkers.Replace(exported), html.UnescapeString)
}
