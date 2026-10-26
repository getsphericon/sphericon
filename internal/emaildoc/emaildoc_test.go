package emaildoc_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden HTML from the current render")

// One runtime serves the whole test process: starting one takes seconds. Tests that must not
// start a runtime use a Renderer of their own, which stays unstarted.
func shared() *emaildoc.Renderer { return emaildoc.Default() }

type node = map[string]any

func text(s string) node { return node{"type": "text", "text": s} }

func paragraph(s string) node {
	return node{"type": "paragraph", "content": []any{text(s)}}
}

func doc(content ...any) []byte {
	out, err := json.Marshal(node{"type": "doc", "content": content})
	if err != nil {
		panic(err)
	}
	return out
}

func readDoc(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "docs", name))
	require.NoError(t, err)
	return b
}

// acceptGolden rewrites a golden file when the test runs with -update.
func acceptGolden(t *testing.T, path, html string) {
	t.Helper()
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(html), 0o644))
	}
}

func TestRenderMatchesGolden(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "docs", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".json")
		t.Run(name, func(t *testing.T) {
			got, err := shared().Render(context.Background(), readDoc(t, name+".json"))
			require.NoError(t, err)

			golden := filepath.Join("testdata", "golden", name+".html")
			acceptGolden(t, golden, got.HTML)
			want, err := os.ReadFile(golden)
			require.NoError(t, err)
			require.Equal(t, string(want), got.HTML,
				"the render changed: a dependency update altered the editor's output (rerun with -update to accept it)")
			require.NotEmpty(t, got.Text)
		})
	}
}

func TestRenderKeepsLiquidTags(t *testing.T) {
	got, err := shared().Render(context.Background(), readDoc(t, "newsletter.json"))
	require.NoError(t, err)
	for _, tag := range []string{"{{ first_name }}", "{{ contact.id }}", "{{ token | url_encode }}", "{{ cta_url }}", "{{ unsubscribe_url }}"} {
		require.Contains(t, got.HTML, tag)
	}
}

func TestRuntimeStartsOnFirstRender(t *testing.T) {
	r := emaildoc.New()
	require.False(t, r.Started(), "a new Renderer starts no runtime")

	// Refused before a runtime is needed.
	_, err := r.Render(context.Background(), doc(node{"type": "heading", "attrs": node{"level": 9}}))
	require.ErrorIs(t, err, emaildoc.ErrInvalid)
	require.False(t, r.Started())

	require.NoError(t, shared().Validate(context.Background(), readDoc(t, "newsletter.json")))
	require.True(t, shared().Started())
}

func TestWarmStartsTheRuntimeWithoutACall(t *testing.T) {
	r := emaildoc.New()

	r.Warm(t.Context())

	require.Eventually(t, r.Started, 2*time.Minute, 50*time.Millisecond, "the runtime starts in the background")
}

func TestLimitsRefuseBeforeTheRuntime(t *testing.T) {
	r := emaildoc.New()
	ctx := context.Background()

	nested := func(depth int) []byte {
		var inner any = paragraph("deep")
		for range depth {
			inner = node{"type": "blockquote", "content": []any{inner}}
		}
		return doc(inner)
	}
	many := make([]any, 4000)
	for i := range many {
		many[i] = paragraph("x")
	}

	cases := []struct {
		name string
		doc  []byte
		want error
	}{
		{"oversized", doc(paragraph(strings.Repeat("a", 300<<10))), emaildoc.ErrTooLarge},
		{"five megabytes of text", doc(paragraph(strings.Repeat("a", 5<<20))), emaildoc.ErrTooLarge},
		{"too deep", nested(40), emaildoc.ErrTooDeep},
		{"absurdly deep JSON", []byte(strings.Repeat(`{"type":"blockquote","content":[`, 7000)), emaildoc.ErrTooDeep},
		{"a hundred thousand brackets", []byte(strings.Repeat("[", 100000)), emaildoc.ErrTooDeep},
		{"too wide", doc(many...), emaildoc.ErrTooManyNodes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.Render(ctx, tc.doc)
			require.ErrorIs(t, err, tc.want)
			require.False(t, r.Started())
		})
	}
}

func TestValidationGivesReasons(t *testing.T) {
	link := func(attrs node) node {
		return node{"type": "text", "text": "go", "marks": []any{node{"type": "link", "attrs": attrs}}}
	}
	cases := []struct {
		name   string
		doc    []byte
		reason []string
	}{
		{
			"heading level 9",
			doc(node{"type": "heading", "attrs": node{"level": 9}, "content": []any{text("Hi")}}),
			[]string{"heading", "level", "1 to 6", "9"},
		},
		{
			"heading level as text",
			doc(node{"type": "heading", "attrs": node{"level": "2"}, "content": []any{text("Hi")}}),
			[]string{"level", "whole number"},
		},
		{
			"button with url and label attributes",
			doc(node{"type": "button", "attrs": node{"url": "https://example.com", "label": "Go"}}),
			[]string{"button", `no attribute "label"`, `"href"`},
		},
		{
			"button without href",
			doc(node{"type": "button", "content": []any{text("Go")}}),
			[]string{"button needs attrs.href"},
		},
		{
			"link without href",
			doc(node{"type": "paragraph", "content": []any{link(node{})}}),
			[]string{"link needs attrs.href"},
		},
		{
			"unknown node",
			doc(node{"type": "callout", "content": []any{paragraph("x")}}),
			[]string{`unknown node type "callout"`, `"paragraph"`},
		},
		{
			"unknown mark",
			doc(node{"type": "paragraph", "content": []any{node{"type": "text", "text": "x", "marks": []any{node{"type": "glow"}}}}}),
			[]string{`unknown mark type "glow"`, `"bold"`},
		},
		{
			"marks as strings",
			doc(node{"type": "paragraph", "content": []any{node{"type": "text", "text": "x", "marks": []any{"bold"}}}}),
			[]string{"marks[0]", "object"},
		},
		{
			"attribute the editor would drop",
			doc(node{"type": "paragraph", "attrs": node{"align": "center"}, "content": []any{text("x")}}),
			[]string{`no attribute "align"`, "alignment"},
		},
		{
			"alignment outside the allowed values",
			doc(node{"type": "paragraph", "attrs": node{"alignment": "justify"}, "content": []any{text("x")}}),
			[]string{"alignment", "left", "right"},
		},
		{
			"empty text node",
			doc(node{"type": "paragraph", "content": []any{text("")}}),
			[]string{"non-empty"},
		},
		{
			"javascript url in a link",
			doc(node{"type": "paragraph", "content": []any{link(node{"href": "javascript:alert(1)"})}}),
			[]string{"href", "absolute URL"},
		},
		{
			"javascript url hidden by whitespace",
			doc(node{"type": "paragraph", "content": []any{link(node{"href": "java\tscript:alert(1)"})}}),
			[]string{"control character"},
		},
		{
			"javascript url in an image",
			doc(node{"type": "image", "attrs": node{"src": "javascript:alert(1)"}}),
			[]string{"src", "absolute URL"},
		},
		{
			"hostile style",
			doc(node{"type": "paragraph", "attrs": node{"style": "background:url(javascript:alert(1))"}, "content": []any{text("x")}}),
			[]string{"style"},
		},
		{
			"text straight in a list item",
			doc(node{"type": "bulletList", "content": []any{node{"type": "listItem", "content": []any{text("x")}}}}),
			[]string{"does not fit the editor's schema"},
		},
		{
			"not JSON",
			[]byte(`{"type":"doc"`),
			[]string{"not valid JSON"},
		},
		{
			"root is not a doc",
			[]byte(`[]`),
			[]string{"JSON object"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := shared().Validate(context.Background(), tc.doc)
			require.ErrorIs(t, err, emaildoc.ErrInvalid)
			for _, part := range tc.reason {
				require.ErrorContains(t, err, part)
			}

			_, err = shared().Render(context.Background(), tc.doc)
			require.ErrorIs(t, err, emaildoc.ErrInvalid, "Render refuses what Validate refuses")
		})
	}
}

func TestValidDocumentsPass(t *testing.T) {
	for _, name := range []string{"newsletter.json", "layout.json"} {
		require.NoError(t, shared().Validate(context.Background(), readDoc(t, name)), name)
	}
}

func TestHostileTextIsEscapedNotExecuted(t *testing.T) {
	got, err := shared().Render(context.Background(), doc(paragraph(`<script>alert(1)</script><img src=x onerror=alert(1)>`)))
	require.NoError(t, err)
	require.NotContains(t, got.HTML, "<script>")
	require.NotContains(t, got.HTML, "<img src=x")
	require.Contains(t, got.HTML, "&lt;script&gt;")
}

func TestEditorSchemaListsNodesAndMarks(t *testing.T) {
	s := emaildoc.EditorSchema()
	require.Contains(t, s.Nodes, "heading")
	require.Contains(t, s.Nodes, "button")
	require.Contains(t, s.Marks, "link")
	require.Contains(t, s.Listing(), "- heading: content \"inline*\"")
}
