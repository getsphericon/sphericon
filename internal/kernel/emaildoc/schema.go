package emaildoc

import (
	_ "embed"
	"encoding/json"
	"fmt"
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

// editorSchema is the editor's schema, read from the embedded file once. Every call returns the
// same maps, which the rules check documents against, so it stays inside the package: an agent
// is handed copies through AgentSchema.
var editorSchema = sync.OnceValue(func() Schema {
	var loaded Schema
	if err := json.Unmarshal(schemaJSON, &loaded); err != nil {
		panic(fmt.Sprintf("emaildoc: the embedded schema.json is not valid: %v", err))
	}
	// The rules name these types; an editor update that renames one must fail here, not let
	// a rule match nothing.
	for _, node := range []string{nodeDoc, nodeText, nodeHeading, nodeButton, nodeImage, nodeOrderedList, nodeCodeBlock} {
		if _, ok := loaded.Nodes[node]; !ok {
			panic(fmt.Sprintf("emaildoc: the editor's schema has no %q node the rules name", node))
		}
	}
	if _, ok := loaded.Marks[markLink]; !ok {
		panic(fmt.Sprintf("emaildoc: the editor's schema has no %q mark the rules name", markLink))
	}
	return loaded
})

// mustMarshal encodes a default value that was itself decoded from JSON (the bundle's schema), so
// it always encodes.
func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic("emaildoc: a decoded JSON value always marshals: " + err.Error())
	}
	return b
}
