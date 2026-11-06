package preset_test

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
)

const headerHMACPreset = "testdata/custom/header-hmac.yaml"

// genericSample is a request signed with the generic Preset's Standard Webhooks scheme (the same
// one its own first test case carries), with the signature header given.
func genericSample(signature string) (preset.Request, preset.Params, time.Time) {
	header := http.Header{}
	header.Set("Webhook-Id", "msg_1")
	header.Set("Webhook-Timestamp", "1767225600")
	header.Set("Webhook-Signature", signature)
	body := []byte(`{"id":"evt_1","destination":"Alice@Example.com","kind":"bounce","hard":true,"sending_domain":"News.Example.com"}`)
	params := preset.Params{"secret": "whsec_" + base64.StdEncoding.EncodeToString([]byte("sphericon-test-secret-0123456789"))}
	return preset.Request{Header: header, Body: body}, params, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func openPreset(t *testing.T, arg string) *preset.Preset {
	t.Helper()
	p, err := preset.Open(arg)
	require.NoError(t, err)
	return p
}

func TestWriteTestsPrintsPassForEachCaseOfACustomFile(t *testing.T) {
	p := openPreset(t, headerHMACPreset)
	var out bytes.Buffer

	err := p.WriteTests(t.Context(), &out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "PASS a signed request passes")
	assert.Contains(t, out.String(), "PASS a signature over another body is refused")
	assert.NotContains(t, out.String(), "FAIL")
}

func TestOpenFindsABuiltInByName(t *testing.T) {
	p := openPreset(t, "generic")
	var out bytes.Buffer

	err := p.WriteTests(t.Context(), &out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "PASS a signed notice becomes a Delivery notice")
}

func TestOpenRefusesANameThatIsNeitherAFileNorABuiltIn(t *testing.T) {
	_, err := preset.Open("no-such-preset")

	require.ErrorContains(t, err, "neither a file nor a built-in Preset")
}

func TestWriteTestsFailsAndNamesTheCaseThatDisagrees(t *testing.T) {
	data, err := os.ReadFile(headerHMACPreset)
	require.NoError(t, err)
	broken := filepath.Join(t.TempDir(), "broken.yaml")
	edited := bytes.Replace(data, []byte("expect: { refused: unauthentic }"), []byte("expect: { notices: [] }"), 1)
	require.NoError(t, os.WriteFile(broken, edited, 0o600))
	p := openPreset(t, broken)
	var out bytes.Buffer

	err = p.WriteTests(t.Context(), &out)

	require.Error(t, err)
	assert.Contains(t, out.String(), "FAIL a signature over another body is refused")
	assert.Contains(t, out.String(), "PASS a signed request passes")
}

func TestOpenReportsAFileThatDoesNotLoad(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("version: 1\nname: bad\n"), 0o600))

	_, err := preset.Open(bad)

	require.ErrorContains(t, err, "bad.yaml")
}

func TestWriteSamplePrintsTheDeliveryNotices(t *testing.T) {
	req, params, now := genericSample("v1,o8DDYiIohO1Z+Or1krVV/dGfR+sT1Gi6NDRUuQ/GbNs=")
	p := openPreset(t, "generic")
	var out bytes.Buffer

	err := p.WriteSample(t.Context(), &out, req, params, now)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "bounce hard alice@example.com")
	assert.Contains(t, out.String(), "id=evt_1")
	assert.Contains(t, out.String(), "sending_domain=news.example.com")
}

// A sample of the chat Preset (testdata/custom/chat.yaml) carries one fact of each kind it maps.
func TestWriteSamplePrintsInboundMessagesAndReachabilityChanges(t *testing.T) {
	p := openPreset(t, "testdata/custom/chat.yaml")
	header := http.Header{}
	header.Set("X-Token", "chat-token")
	body := `{"events":[{"type":"message","event_id":"7","from":42,"chat":43,"message":11,"text":"/stop"},` +
		`{"type":"button","event_id":"8","from":42,"chat":43,"message":12,"data":"unsubscribe"},` +
		`{"type":"blocked","event_id":"9","from":42},{"type":"unblocked","event_id":"10","from":44}]}`
	var out bytes.Buffer

	err := p.WriteSample(t.Context(), &out, preset.Request{Header: header, Body: []byte(body)}, preset.Params{"token": "chat-token"}, time.Now())

	require.NoError(t, err)
	assert.Equal(t, `inbound_message telegram 42 id=7 chat_id=43 message_id=11 text="/stop" payload=""
inbound_message telegram 42 id=8 chat_id=43 message_id=12 text="" payload="unsubscribe"
reachability_change 42 id=9 unreachable reason=blocked
reachability_change 44 id=10 reachable
`, out.String())
}

func TestWriteSampleSaysWhenAVerifiedRequestCarriesNoFacts(t *testing.T) {
	p := openPreset(t, "testdata/custom/chat.yaml")
	header := http.Header{}
	header.Set("X-Token", "chat-token")
	var out bytes.Buffer

	err := p.WriteSample(t.Context(), &out, preset.Request{Header: header, Body: []byte(`{"events":[]}`)}, preset.Params{"token": "chat-token"}, time.Now())

	require.NoError(t, err)
	assert.Equal(t, "verified, no facts (an event the Preset does not handle)\n", out.String())
}

func TestWriteSamplePrintsTheReasonOfARefusal(t *testing.T) {
	req, params, now := genericSample("v1,wrong")
	p := openPreset(t, "generic")
	var out bytes.Buffer

	err := p.WriteSample(t.Context(), &out, req, params, now)

	require.Error(t, err)
	assert.Contains(t, out.String(), "refused: ")
	assert.Contains(t, out.String(), "not authentic")
}
