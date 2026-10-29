package testhelper

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"
	"sync"

	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// RendererEnv set to "goja" makes Renderer the real one: the nightly suite runs the save paths
// through the editor's own export, which a pull request leaves to internal/emaildoc and
// internal/emailrender.
const RendererEnv = "TEST_EMAIL_RENDERER"

var (
	fakeRenderer = sync.OnceValue(func() *emaildoc.Renderer {
		return emaildoc.New(emaildoc.WithEngine(fakeEmailEngine{}))
	})
	realRenderer = sync.OnceValue(func() *emaildoc.Renderer { return emaildoc.New() })
)

// Renderer is the document renderer of the test process's save paths. A runtime takes seconds to
// start, many more on a busy machine, so it is a fake that exports a document's text as paragraphs:
// our own checks still run, the editor's schema check and export do not. RendererEnv=goja makes
// it the real one.
func Renderer() *emaildoc.Renderer {
	if os.Getenv(RendererEnv) == "goja" {
		return realRenderer()
	}
	return fakeRenderer()
}

// RealRenderer is the editor's own export, one runtime per test process, for a test that checks
// what the export makes of a document.
func RealRenderer() *emaildoc.Renderer { return realRenderer() }

// fakeEmailEngine accepts every document that passed our checks and exports each block's text as
// an escaped paragraph, so a stored body is still built from its document.
type fakeEmailEngine struct{}

func (fakeEmailEngine) Check([]byte) error { return nil }

func (fakeEmailEngine) Export(doc []byte) (emaildoc.Rendered, error) {
	var root docNode
	if err := json.Unmarshal(doc, &root); err != nil {
		return emaildoc.Rendered{}, fmt.Errorf("fake export: %w", err)
	}
	var blocks []string
	root.blocks(&blocks)
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head></head><body>")
	for _, text := range blocks {
		b.WriteString("<p>" + html.EscapeString(text) + "</p>")
	}
	b.WriteString("</body></html>")
	return emaildoc.Rendered{HTML: b.String(), Text: strings.Join(blocks, "\n\n")}, nil
}

type docNode struct {
	Type    string    `json:"type"`
	Text    string    `json:"text"`
	Content []docNode `json:"content"`
}

// blocks appends the text of every node whose children are all text nodes, in document order.
func (n docNode) blocks(out *[]string) {
	var text strings.Builder
	inline := len(n.Content) > 0
	for _, c := range n.Content {
		if c.Type != "text" {
			inline = false
			break
		}
		text.WriteString(c.Text)
	}
	if inline {
		*out = append(*out, text.String())
		return
	}
	for _, c := range n.Content {
		c.blocks(out)
	}
}
