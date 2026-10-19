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
// A document that is invalid, too large or does not render within its limits is
// ErrInvalidContent with the reason; subject, when not nil, is checked with the body. A failure
// of the runtime itself is returned as it is.
func FromDocument(ctx context.Context, subject *string, doc []byte) (Authored, error) {
	out, err := emaildoc.Default().Render(ctx, doc)
	if err != nil {
		if isDocumentRefusal(err) {
			return Authored{}, fmt.Errorf("%w: %w", ErrInvalidContent, err)
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

// isDocumentRefusal tells what the author can fix (the document) from what they cannot.
func isDocumentRefusal(err error) bool {
	for _, target := range []error{
		emaildoc.ErrInvalid, emaildoc.ErrTooLarge, emaildoc.ErrTooDeep, emaildoc.ErrTooManyNodes,
		emaildoc.ErrTimeout, emaildoc.ErrMemoryLimit,
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
