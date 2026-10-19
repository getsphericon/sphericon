package emaildoc

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// schemaJSON is the listing of the editor's nodes and marks, derived from the same extensions that
// render, by the bundle build (`mise run build:emaildoc`). It is generated: regenerate, never edit.
//
//go:embed assets/schema.json
var schemaJSON []byte

// editorVersionText is the version of @react-email/editor the bundle was built from, written by the
// bundle build from the installed package (the lockfile's pin). It is generated: never edit.
//
//go:embed assets/editor-version.txt
var editorVersionText string

// EditorVersion is the version of the editor whose export code the renderer runs. A stored document
// records it, so a later dependency update can tell which documents it has to re-render.
func EditorVersion() string { return strings.TrimSpace(editorVersionText) }

// Schema is what the editor's extensions define: every node and mark with its content expression
// and the attributes it accepts, with their defaults. ProseMirror keeps no attribute types or
// allowed values, so those live in this package's rules and in what an agent is told by hand.
type Schema struct {
	Nodes map[string]SchemaType `json:"nodes"`
	Marks map[string]SchemaType `json:"marks"`
}

// SchemaType is one node or mark.
type SchemaType struct {
	// Content is the ProseMirror content expression ("paragraph block*"); empty for a leaf.
	Content string `json:"content,omitempty"`
	// Group is the group the node belongs to ("block", "inline"); empty for a mark.
	Group string `json:"group,omitempty"`
	// Attrs maps each accepted attribute to its default.
	Attrs map[string]any `json:"attrs"`
}

var (
	schemaOnce   sync.Once
	loadedSchema Schema
)

// EditorSchema returns the editor's schema listing.
func EditorSchema() Schema {
	schemaOnce.Do(func() {
		if err := json.Unmarshal(schemaJSON, &loadedSchema); err != nil {
			panic(fmt.Sprintf("emaildoc: the embedded schema.json is not valid: %v", err))
		}
	})
	return loadedSchema
}

// Listing is the schema as compact text for an agent: one line per node and mark.
func (s Schema) Listing() string {
	var b strings.Builder
	b.WriteString("Nodes:\n")
	writeTypes(&b, s.Nodes)
	b.WriteString("Marks:\n")
	writeTypes(&b, s.Marks)
	return b.String()
}

func writeTypes(b *strings.Builder, types map[string]SchemaType) {
	for _, name := range sortedKeys(types) {
		t := types[name]
		fmt.Fprintf(b, "- %s", name)
		if t.Content != "" {
			fmt.Fprintf(b, ": content %q", t.Content)
		}
		if len(t.Attrs) > 0 {
			attrs := make([]string, 0, len(t.Attrs))
			for _, attr := range sortedKeys(t.Attrs) {
				def, _ := json.Marshal(t.Attrs[attr])
				attrs = append(attrs, attr+"="+string(def))
			}
			fmt.Fprintf(b, "; attrs %s", strings.Join(attrs, ", "))
		}
		b.WriteString("\n")
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
