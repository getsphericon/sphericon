package emailrender_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/emailrender"
)

// The editor, given a theme (the Brand kit's palette, ADR 0040), writes it into the document as a
// globalContent node when a message is first authored. The server's render reads it back from the
// document, so the brand reaches the stored body without anything resolving it at send time.
func TestFromDocumentRendersTheThemeTheEditorWroteIntoTheDocument(t *testing.T) {
	doc := `{"type":"doc","content":[
	  {"type":"globalContent","attrs":{"data":{"theme":"basic","styles":[
	    {"id":"body","title":"Body","inputs":[
	      {"label":"Background","type":"color","prop":"backgroundColor","classReference":"body","value":"#abcdef"}]},
	    {"id":"button","title":"Button","inputs":[
	      {"label":"Background","type":"color","prop":"backgroundColor","classReference":"button","value":"#ff5500"}]}
	  ]}}},
	  {"type":"paragraph","content":[{"type":"text","text":"Hello"}]}]}`

	got, err := emailrender.FromDocument(t.Context(), renderer, nil, []byte(doc))
	require.NoError(t, err)

	assert.Contains(t, got.Body, "#abcdef")
	assert.Equal(t, emailrender.SanitizeHTML(got.Body), got.Body, "the stored body survives the sanitizer")
}

// editor_branded.json is a document as the editor emitted it for a new message in a Brand kit's
// theme (captured from the Template form's test), panels and all.
func TestFromDocumentAcceptsADocumentThemedByTheEditorWithABrandKit(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("testdata", "editor_branded.json"))
	require.NoError(t, err)

	got, err := emailrender.FromDocument(t.Context(), renderer, nil, doc)
	require.NoError(t, err)

	for _, brand := range []string{"#f0f4f8", "#ff5500", "#fefefe", "border-radius:12px", "Georgia, serif", "Verdana, sans-serif"} {
		assert.Contains(t, got.Body, brand)
	}
	assert.Contains(t, got.Body, "Hello {{ first_name }}")
}
