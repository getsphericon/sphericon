package preset_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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
	f.Add(`[{"id":"SM1/undelivered","channel":"sms","destination":" +14155552671 ","kind":"drop","status":"undelivered","error_code":"30003"}]`)
	f.Add(`[{"id":"n1","channel":"sms","destination":"","kind":"delivery"}]`)
	f.Add(`{}`)

	f.Fuzz(func(t *testing.T, body string) {
		res, err := handlePassThrough(t, body)

		assertValidFactsOrARefusal(t, res, err)
	})
}

// twilioHookURL is where the fuzzed Twilio callbacks are posted.
const twilioHookURL = "https://sphericon.example/hooks/omik_fuzz"

// signTwilio signs a form body as Twilio does ("Webhooks security"), so the fuzzer's bodies pass
// verification and reach the form parser and the transform.
func signTwilio(rawURL, body, token string) string {
	values, err := url.ParseQuery(body)
	if err != nil {
		// An unparsable body is refused as malformed before its signature is read.
		return ""
	}
	content := []byte(rawURL)
	for _, name := range slices.Sorted(maps.Keys(values)) {
		for _, v := range slices.Sorted(slices.Values(values[name])) {
			content = append(content, name+v...)
		}
	}
	mac := hmac.New(sha1.New, []byte(token))
	mac.Write(content)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// Whatever a signed form-encoded callback holds, the built-in twilio Preset answers with valid
// facts or a refusal of a known kind, never both and never a panic.
func FuzzTwilioFormBodyBecomesValidFactsOrARefusal(f *testing.F) {
	f.Add("MessageSid=SM1&MessageStatus=undelivered&ErrorCode=30003&To=%2B14155552671&From=%2B15017250604")
	f.Add("MessageSid=SM2&Body=STOP&From=%2B14155552671&To=%2B15017250604&SmsStatus=received")
	f.Add("MessageSid=SM3&MessageSid=SM4&Body=a&Body=b&From=%2B1")
	f.Add("MessageSid=SM5&MessageStatus=&To=+")
	f.Add("Body=%F0%9F%91%8D&From=%20%2B1%20&MessageSid=" + strings.Repeat("x", 300))
	f.Add("a=%zz")
	f.Add("")
	reg, err := preset.Load(nil)
	if err != nil {
		f.Fatal(err)
	}
	p, ok := reg.Get("twilio")
	if !ok {
		f.Fatal("the twilio Preset is not built in")
	}
	const token = "fuzz-auth-token"

	f.Fuzz(func(t *testing.T, body string) {
		header := http.Header{}
		header.Set("Content-Type", "application/x-www-form-urlencoded")
		header.Set("X-Twilio-Signature", signTwilio(twilioHookURL, body, token))
		req := preset.Request{URL: twilioHookURL, Header: header, Body: []byte(body)}

		res, err := p.Handle(t.Context(), req, preset.Params{"auth_token": token}, time.Unix(0, 0))

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
	for i := range res.Notices {
		n := &res.Notices[i]
		assert.NotEmpty(t, n.ID)
		assert.NotEmpty(t, n.Channel, "a notice always has its Channel")
		assert.NotEmpty(t, n.Destination)
		assert.Equal(t, strings.TrimSpace(n.Destination), n.Destination)
	}
	for i := range res.InboundMessages {
		m := &res.InboundMessages[i]
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
