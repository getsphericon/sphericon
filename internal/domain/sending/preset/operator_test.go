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

// passed names the cases of results that passed, and failed those that did not.
func outcomes(results []preset.CaseResult) (passed, failed []string) {
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, r.Name)
			continue
		}
		passed = append(passed, r.Name)
	}
	return passed, failed
}

func TestRunTestsPassesEachCaseOfACustomFile(t *testing.T) {
	p := openPreset(t, headerHMACPreset)

	passed, failed := outcomes(p.RunTests(t.Context()))

	assert.Contains(t, passed, "a signed request passes")
	assert.Contains(t, passed, "a signature over another body is refused")
	assert.Empty(t, failed)
}

func TestOpenFindsABuiltInByName(t *testing.T) {
	p := openPreset(t, "generic")

	passed, failed := outcomes(p.RunTests(t.Context()))

	assert.Contains(t, passed, "a signed notice becomes a Delivery notice")
	assert.Empty(t, failed)
}

func TestOpenRefusesANameThatIsNeitherAFileNorABuiltIn(t *testing.T) {
	_, err := preset.Open("no-such-preset")

	require.ErrorContains(t, err, "neither a file nor a built-in Preset")
}

func TestRunTestsNamesTheCaseThatDisagrees(t *testing.T) {
	data, err := os.ReadFile(headerHMACPreset)
	require.NoError(t, err)
	broken := filepath.Join(t.TempDir(), "broken.yaml")
	edited := bytes.Replace(data, []byte("expect: { refused: unauthentic }"), []byte("expect: { notices: [] }"), 1)
	require.NoError(t, os.WriteFile(broken, edited, 0o600))
	p := openPreset(t, broken)

	passed, failed := outcomes(p.RunTests(t.Context()))

	assert.Equal(t, []string{"a signature over another body is refused"}, failed)
	assert.Contains(t, passed, "a signed request passes")
}

func TestOpenReportsAFileThatDoesNotLoad(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("version: 1\nname: bad\n"), 0o600))

	_, err := preset.Open(bad)

	require.ErrorContains(t, err, "bad.yaml")
}

func TestHandleOfASampleMakesTheDeliveryNotices(t *testing.T) {
	req, params, now := genericSample("v1,o8DDYiIohO1Z+Or1krVV/dGfR+sT1Gi6NDRUuQ/GbNs=")
	p := openPreset(t, "generic")

	res, err := p.Handle(t.Context(), req, params, now)

	require.NoError(t, err)
	require.Len(t, res.Notices, 1)
	assert.Equal(t, "evt_1", res.Notices[0].ID)
	assert.Equal(t, "alice@example.com", res.Notices[0].Destination)
	assert.Equal(t, preset.Bounce, res.Notices[0].Kind)
	assert.True(t, res.Notices[0].Hard)
	assert.Equal(t, "news.example.com", res.Notices[0].SendingDomain)
}

// A sample of the chat Preset (testdata/custom/chat.yaml) carries one fact of each kind it maps.
func TestHandleOfASampleMakesInboundMessagesAndReachabilityChanges(t *testing.T) {
	p := openPreset(t, "testdata/custom/chat.yaml")
	header := http.Header{}
	header.Set("X-Token", "chat-token")
	body := `{"events":[{"type":"message","event_id":"7","from":42,"chat":43,"message":11,"text":"/stop"},` +
		`{"type":"button","event_id":"8","from":42,"chat":43,"message":12,"data":"unsubscribe"},` +
		`{"type":"blocked","event_id":"9","from":42},{"type":"unblocked","event_id":"10","from":44}]}`

	res, err := p.Handle(t.Context(), preset.Request{Header: header, Body: []byte(body)}, preset.Params{"token": "chat-token"}, time.Now())

	require.NoError(t, err)
	assert.Equal(t, []preset.InboundMessage{
		{ID: "7", Channel: "telegram", Destination: "42", ChatID: "43", MessageID: "11", Text: "/stop"},
		{ID: "8", Channel: "telegram", Destination: "42", ChatID: "43", MessageID: "12", Payload: "unsubscribe"},
	}, res.InboundMessages)
	assert.Equal(t, []preset.ReachabilityChange{
		{ID: "9", Destination: "42", Reason: preset.Blocked},
		{ID: "10", Destination: "44", Reachable: true},
	}, res.ReachabilityChanges)
}

func TestHandleOfAVerifiedRequestWithoutFactsIsEmpty(t *testing.T) {
	p := openPreset(t, "testdata/custom/chat.yaml")
	header := http.Header{}
	header.Set("X-Token", "chat-token")

	res, err := p.Handle(t.Context(), preset.Request{Header: header, Body: []byte(`{"events":[]}`)}, preset.Params{"token": "chat-token"}, time.Now())

	require.NoError(t, err)
	assert.True(t, res.Empty())
}

func TestHandleRefusesASampleThatIsNotAuthentic(t *testing.T) {
	req, params, now := genericSample("v1,wrong")
	p := openPreset(t, "generic")

	_, err := p.Handle(t.Context(), req, params, now)

	require.ErrorContains(t, err, "not authentic")
}
