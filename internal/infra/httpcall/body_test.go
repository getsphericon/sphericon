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
