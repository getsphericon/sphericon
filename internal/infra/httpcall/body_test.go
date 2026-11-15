package httpcall_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/httpcall"
)

// hostile is a value a Contact controls that would break the JSON around it if written raw.
const hostile = "say \"hi\" \\ to <a & b>\nline end"

// decoded renders body against bindings and reads the JSON it renders to.
func decoded(t *testing.T, body string, bindings map[string]any) map[string]any {
	t.Helper()
	out, err := httpcall.RenderBody(body, bindings)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	return got
}

func TestRenderBodyWritesAStringValueInsideAJSONStringAsItIs(t *testing.T) {
	got := decoded(t, `{"v": "{{ v }}"}`, map[string]any{"v": hostile})

	assert.Equal(t, map[string]any{"v": hostile}, got)
}

func TestRenderBodyEscapesTheStringsOfNestedValues(t *testing.T) {
	bindings := map[string]any{
		"contact": map[string]any{"name": hostile},
		"tags":    []any{hostile, "plain"},
	}
	got := decoded(t, `{"name": "{{ contact.name }}", "first": "{{ tags[0] }}", "all": [{% for t in tags %}"{{ t }}"{% unless forloop.last %},{% endunless %}{% endfor %}]}`, bindings)

	assert.Equal(t, map[string]any{"name": hostile, "first": hostile, "all": []any{hostile, "plain"}}, got)
}

func TestRenderBodyWritesANumberAsANumber(t *testing.T) {
	got := decoded(t, `{"n": {{ n }}, "ok": {{ ok }}}`, map[string]any{"n": 42, "ok": true})

	assert.Equal(t, map[string]any{"n": 42.0, "ok": true}, got)
}

func TestRenderBodyRefusesWhatIsNotJSON(t *testing.T) {
	_, err := httpcall.RenderBody(`{"v": {{ v }}}`, map[string]any{"v": "not a number"})

	require.ErrorIs(t, err, httpcall.ErrInvalidBody)
}

func TestRenderBodyFiltersAndComparesTheValueNotItsEscapedForm(t *testing.T) {
	body := `{"up": "{{ v | upcase }}", "size": {{ v | size }}, "same": {% if v == 'a"b' %}true{% else %}false{% endif %}}`
	got := decoded(t, body, map[string]any{"v": "a\"b"})

	assert.Equal(t, map[string]any{"up": "A\"B", "size": 3.0, "same": true}, got)
}

func TestRenderBodyEscapesWhatAFilterAdds(t *testing.T) {
	got := decoded(t, `{"v": "{{ v | append: '"' }}", "d": "{{ missing | default: 'x"y' }}"}`, map[string]any{"v": "a"})

	assert.Equal(t, map[string]any{"v": "a\"", "d": "x\"y"}, got)
}

// A captured block is escaped as it is captured, so its output takes `safe` to stay escaped once.
func TestRenderBodyWritesACapturedBlockRawThroughSafe(t *testing.T) {
	got := decoded(t, `{% capture c %}{{ v }}!{% endcapture %}{"v": "{{ c | safe }}"}`, map[string]any{"v": hostile})

	assert.Equal(t, map[string]any{"v": hostile + "!"}, got)
}

// The json filter writes JSON text, which the body takes raw through `safe`; its strings are
// encoded once, by the filter.
func TestRenderBodyWritesAnObjectThroughJSONAndSafe(t *testing.T) {
	props := map[string]any{"name": hostile, "n": 2}
	got := decoded(t, `{"props": {{ props | json | safe }}}`, map[string]any{"props": props})

	assert.Equal(t, map[string]any{"props": map[string]any{"name": hostile, "n": 2.0}}, got)
}
