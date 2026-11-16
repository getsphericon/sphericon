package emailrender_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
)

// ADR 0174: what a render may loop and build is bounded, so a template written to exhaust the
// process every Workspace shares fails as its own content instead. Each of these would run for
// minutes or take gigabytes unbounded; each ends at once.
func TestARenderThatDoesTooMuchWorkFails(t *testing.T) {
	for name, tmpl := range map[string]string{
		"doubling by append":          `{% assign a = "x" %}{% for i in (1..40) %}{% assign a = a | append: a %}{% endfor %}{{ a | size }}`,
		"doubling by capture":         `{% assign a = "x" %}{% for i in (1..40) %}{% capture a %}{{a}}{{a}}{% endcapture %}{% endfor %}{{ a | size }}`,
		"doubling an array by concat": `{% assign a = "x" | split: "," %}{% for i in (1..40) %}{% assign a = a | concat: a %}{% endfor %}{{ a | size }}`,
		"nested empty loops":          `{% for i in (1..1000) %}{% for j in (1..1000) %}{% endfor %}{% endfor %}`,
		"a loop over a split":         `{% assign s = "x" %}{% for i in (1..17) %}{% assign s = s | append: s %}{% endfor %}{% for c in s | split: "" %}{% endfor %}`,
		"a replace that multiplies":   `{% capture a %}{% for i in (1..500) %}aaaaaaaaaa{% endfor %}{% endcapture %}{{ a | replace: "a", a }}`,
		"a join with a large separator": `{% capture sep %}{% for i in (1..500) %}0123456789{% endfor %}{% endcapture %}` +
			`{{ (1..1000) | join: sep }}`,
		"output past the bound": `{% capture a %}{% for i in (1..500) %}0123456789{% endfor %}{% endcapture %}` +
			`{% for i in (1..1000) %}{{ a }}{% endfor %}`,
		"a range wider than an array may be": `{% for i in (1..1000000000) %}{% endfor %}`,
		"a range with a bound it reads":      `{% for i in (1..n) %}x{% endfor %}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := emailrender.RenderBody(tmpl, map[string]any{"n": 3})
			require.ErrorIs(t, err, emailrender.ErrTooMuchWork)
		})
	}
}

// The bounds leave an ordinary message alone: loops over a contact's data, assigns, captures,
// filters, break and continue, the else of an empty loop, tablerow and cycle render as before.
func TestABoundedRenderRendersOrdinaryTemplatesAsBefore(t *testing.T) {
	bindings := map[string]any{
		"contact": map[string]any{"first_name": "Ada", "tags": []any{"vip", "beta", "news"}},
		"lines":   []any{map[string]any{"name": "Tea", "qty": 2}, map[string]any{"name": "Cake", "qty": 1}},
		"none":    []any{},
	}
	for tmpl, want := range map[string]string{
		`Hi {{ contact.first_name | upcase }}`:                                                           "Hi ADA",
		`{% for l in lines %}{{ forloop.index }}.{{ l.name }}x{{ l.qty }} {% endfor %}`:                  "1.Teax2 2.Cakex1 ",
		`{% for t in contact.tags %}{% if t == "beta" %}{% continue %}{% endif %}{{ t }} {% endfor %}`:   "vip news ",
		`{% for t in contact.tags %}{% if forloop.index > 1 %}{% break %}{% endif %}{{ t }}{% endfor %}`: "vip",
		`{% for t in none %}x{% else %}empty{% endfor %}`:                                                "empty",
		`{% for i in (1..3) reversed %}{{ i }}{% endfor %}`:                                              "321",
		`{% for t in contact.tags limit: 2 offset: 1 %}{{ t }},{% endfor %}`:                             "beta,news,",
		`{% for t in contact.tags %}{% cycle "a", "b" %}{% endfor %}`:                                    "aba",
		`{% tablerow t in contact.tags cols: 2 %}{{ t }}{% endtablerow %}`:                               `<tr class="row1"><td class="col1">vip</td><td class="col2">beta</td></tr><tr class="row2"><td class="col1">news</td></tr>`,
		`{% assign n = contact.tags | size %}{% capture c %}{{ n }} tags{% endcapture %}{{ c }}`:         "3 tags",
		`{{ contact.tags | join: ", " | prepend: "[" | append: "]" }}`:                                   "[vip, beta, news]",
		`{{ "<b>" }}{{ "<b>" | safe }}`:                                                                  "&lt;b&gt;<b>",
		`{{ product.title | truncate: 8, "..." }}`:                                                       "",
	} {
		got, err := emailrender.RenderBody(tmpl, bindings)
		require.NoError(t, err, tmpl)
		assert.Equal(t, want, got, tmpl)
	}
}

// The bound is per render: one render's steps never count against the next.
func TestEachRenderHasItsOwnBudget(t *testing.T) {
	tmpl := `{% for i in (1..1000) %}{% for j in (1..4) %}{% endfor %}{% endfor %}done`

	for range 3 {
		got, err := emailrender.RenderBody(tmpl, nil)
		require.NoError(t, err)
		assert.Equal(t, "done", got)
	}
	_, err := emailrender.RenderBody(strings.Replace(tmpl, "(1..4)", "(1..5)", 1), nil)
	require.ErrorIs(t, err, emailrender.ErrTooMuchWork)
}

// Whatever an author writes, a render returns text or an error, never panics (the library's own
// panics are ErrUnparsable), and never writes past its bound.
func FuzzABoundedRender(f *testing.F) {
	for _, seed := range []string{
		`{% for i in (1..3) %}{{ i | append: "x" | replace: "x", "yy" }}{% endfor %}`,
		`{% assign a = "x" %}{% capture b %}{{ a }}{% endcapture %}{{ b | join: "," }}`,
		`{% case n %}{% when 1 %}one{% else %}other{% endcase %}{% cycle "a", "b" %}`,
		`{{ (1..1000) | join: "," | split: "," | reverse | first }}`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, tmpl string) {
		out, err := emailrender.RenderBody(tmpl, map[string]any{"n": 1, "contact": map[string]any{"tags": []any{"a"}}})
		assert.LessOrEqual(t, len(out), emailrender.MaxOutputBytes)
		assert.False(t, out != "" && err != nil, "text or an error, never both")
	})
}
