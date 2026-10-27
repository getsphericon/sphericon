package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const headerHMACPreset = "../../internal/preset/testdata/custom/header-hmac.yaml"

// A sample signed with the generic Preset's Standard Webhooks scheme (the same one its own
// first test case carries).
var genericSecret = "secret=whsec_" + base64.StdEncoding.EncodeToString([]byte("sphericon-test-secret-0123456789"))

const (
	genericPayload = `{"id":"evt_1","destination":"Alice@Example.com","kind":"bounce","permanent":true,"sending_domain":"News.Example.com"}`
)

func genericArgs(payload string, extra ...string) []string {
	return append([]string{
		"preset", "test", "generic", "--payload", payload,
		"--now", "2026-01-01T00:00:00Z",
		"--param", genericSecret,
		"--header", "webhook-id=msg_1",
		"--header", "webhook-timestamp=1767225600",
	}, extra...)
}

func samplePayload(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "payload.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

func TestPresetSchemaPrintsThePublishedSchema(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, execute(nil, &out, "preset", "schema"))

	var schema map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &schema))
	assert.Equal(t, "https://sphericon.app/schemas/preset-1.schema.json", schema["$id"])
}

func TestPresetTestPrintsPassForEachCaseOfACustomFile(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, execute(nil, &out, "preset", "test", headerHMACPreset))

	assert.Contains(t, out.String(), "PASS a signed request passes")
	assert.Contains(t, out.String(), "PASS a signature over another body is refused")
	assert.NotContains(t, out.String(), "FAIL")
}

func TestPresetTestRunsABuiltInByName(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, execute(nil, &out, "preset", "test", "generic"))

	assert.Contains(t, out.String(), "PASS a signed notice becomes a Delivery notice")
}

func TestPresetTestFailsAndNamesTheCaseThatDisagrees(t *testing.T) {
	data, err := os.ReadFile(headerHMACPreset)
	require.NoError(t, err)
	broken := filepath.Join(t.TempDir(), "broken.yaml")
	edited := bytes.Replace(data, []byte("expect: { refused: unauthentic }"), []byte("expect: { notices: [] }"), 1)
	require.NoError(t, os.WriteFile(broken, edited, 0o600))
	var out bytes.Buffer

	err = execute(nil, &out, "preset", "test", broken)

	require.Error(t, err)
	assert.Contains(t, out.String(), "FAIL a signature over another body is refused")
	assert.Contains(t, out.String(), "PASS a signed request passes")
}

func TestPresetTestReportsAFileThatDoesNotLoad(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("version: 1\nname: bad\n"), 0o600))

	err := execute(nil, &bytes.Buffer{}, "preset", "test", bad)

	require.ErrorContains(t, err, "bad.yaml")
}

func TestPresetTestPayloadPrintsTheDeliveryNotices(t *testing.T) {
	var out bytes.Buffer
	args := genericArgs(samplePayload(t, genericPayload), "--header", "webhook-signature=v1,iasTRX51zsWZMikELc3A1LcI3NQUrVXBpWWg+/Hp82I=")

	require.NoError(t, execute(nil, &out, args...))

	assert.Contains(t, out.String(), "bounce permanent alice@example.com")
	assert.Contains(t, out.String(), "evt_1")
	assert.Contains(t, out.String(), "news.example.com")
}

func TestPresetTestPayloadPrintsTheReasonOfARefusal(t *testing.T) {
	var out bytes.Buffer
	args := genericArgs(samplePayload(t, genericPayload), "--header", "webhook-signature=v1,wrong")

	err := execute(nil, &out, args...)

	require.Error(t, err)
	assert.Contains(t, out.String(), "not authentic")
}
