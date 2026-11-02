package preset_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/preset"
)

// A Preset file is operator input: whatever it holds, Parse answers with a Preset or an error,
// never both and never a panic.
func FuzzParseAnswersWithAPresetOrAnError(f *testing.F) {
	for _, pattern := range []string{"builtin/*.yaml", "testdata/custom/*.yaml"} {
		seeds, err := filepath.Glob(filepath.FromSlash(pattern))
		if err != nil {
			f.Fatal(err)
		}
		for _, path := range seeds {
			data, err := os.ReadFile(path)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(data)
		}
	}
	f.Add([]byte(""))
	f.Add([]byte("name: x"))

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := preset.Parse("fuzz.yaml", data)

		require.NotEqualf(t, p == nil, err == nil, "Parse(%q) returned preset %v and error %v", data, p, err)
	})
}

// passThrough is a Preset whose transform returns the body as its facts, so a test (or the fuzzer)
// hands the engine's validation any output a transform could produce.
const passThrough = `
version: 1
name: pass-through
params: {type: object, properties: {token: {type: string, writeOnly: true}}}
verify: {kind: token, secret: token, header: X-Token}
transform: .body
tests:
  - {name: unsigned, request: {body: "[]"}, expect: {refused: unauthentic}}
`

func handlePassThrough(t *testing.T, body string) (preset.Result, error) {
	t.Helper()
	p, err := preset.Parse("pass-through.yaml", []byte(passThrough))
	require.NoError(t, err)
	header := http.Header{}
	header.Set("X-Token", "t")
	return p.Handle(t.Context(), preset.Request{Header: header, Body: []byte(body)}, preset.Params{"token": "t"}, time.Unix(0, 0))
}

// What a transform returns is never trusted: whatever it is, the engine answers with facts of a
// closed kind, each with an id and a Destination, or refuses the request as malformed or as a
// failed transform; never both, never a panic.
func FuzzTransformOutputBecomesValidFactsOrARefusal(f *testing.F) {
	f.Add(`[{"id":"e1","destination":"Alice@Example.com","kind":"bounce","hard":true}]`)
	f.Add(`[{"kind":"inbound_message","id":"7","channel":"telegram","destination":"42","chat_id":"42","text":"/start abc","message_id":"11"}]`)
	f.Add(`[{"kind":"inbound_message","id":"8","channel":"telegram","destination":"42","payload":"unsubscribe"}]`)
	f.Add(`[{"kind":"reachability_change","id":"9","destination":"42","reachable":false,"reason":"blocked"}]`)
	f.Add(`[{"kind":"reachability_change","id":"10","destination":"42","reachable":true}]`)
	f.Add(`[{"kind":"reachability_change","id":"11","destination":"42","reachable":true,"reason":"deactivated"}]`)
	f.Add(`[{"kind":"inbound_message","id":"12","channel":"pigeon","destination":" "}]`)
	f.Add(`{}`)

	f.Fuzz(func(t *testing.T, body string) {
		res, err := handlePassThrough(t, body)

		assertValidFactsOrARefusal(t, res, err)
	})
}

// assertValidFactsOrARefusal states what holds of any answer: a refusal of a known kind with no
// facts, or at most MaxFacts facts that each keep the rules of their kind.
func assertValidFactsOrARefusal(t *testing.T, res preset.Result, err error) {
	t.Helper()
	if err != nil {
		assert.Truef(t, errors.Is(err, preset.ErrMalformed) || errors.Is(err, preset.ErrTransformFailed), "unexpected error %v", err)
		assert.True(t, res.Empty(), "a refusal carries no facts")
		return
	}
	assert.LessOrEqual(t, len(res.Notices)+len(res.InboundMessages)+len(res.ReachabilityChanges), preset.MaxFacts)
	for _, m := range res.InboundMessages {
		assert.NotEmpty(t, m.ID)
		assert.NotEmpty(t, m.Destination)
		assert.Equal(t, strings.TrimSpace(m.Destination), m.Destination)
		assert.LessOrEqual(t, utf8.RuneCountInString(m.Text), preset.MaxTextChars)
	}
	for _, c := range res.ReachabilityChanges {
		assert.NotEmpty(t, c.ID)
		assert.NotEmpty(t, c.Destination)
		assert.Equalf(t, c.Reachable, c.Reason == "", "a reason exactly when unreachable: %+v", c)
	}
}

// A person's text is theirs: a text over the limit is cut, not refused, so the provider does not
// redeliver the request forever.
func TestAnInboundMessageTextOverTheLimitIsCut(t *testing.T) {
	long := strings.Repeat("é", preset.MaxTextChars+1)

	res, err := handlePassThrough(t, `[{"kind":"inbound_message","id":"1","channel":"telegram","destination":" 42 ","text":"`+long+`"}]`)

	require.NoError(t, err)
	require.Len(t, res.InboundMessages, 1)
	got := res.InboundMessages[0]
	assert.Equal(t, strings.Repeat("é", preset.MaxTextChars), got.Text)
	assert.Equal(t, "42", got.Destination, "the destination is trimmed")
}
