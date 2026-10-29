package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

const customPreset = `version: 1
name: acme-relay
params: {type: object}
verify: {kind: standard_webhooks, secret: secret}
transform: "[]"
tests:
  - {name: unsigned, request: {body: "{}"}, expect: {refused: unauthentic}}
`

func writePresetDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return dir
}

// inProcess sends a client's requests straight to a handler, without a socket.
type inProcess struct{ handler http.Handler }

func (c inProcess) Do(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// bearer is the fixture token that carries every scope.
type bearer struct{}

func (bearer) BearerAuth(context.Context, externalapi.OperationName) (externalapi.BearerAuth, error) {
	return externalapi.BearerAuth{Token: credentials.TokenValue(fixtures.ContractTokenPrefix, fixtures.ContractTokenSecret)}, nil
}

// presetNames lists the Presets the app's /api offers.
func presetNames(t *testing.T, a *app.App) []string {
	t.Helper()
	client, err := externalapi.NewClient("http://local/api", bearer{}, externalapi.WithClient(inProcess{a.Server.Handler}))
	require.NoError(t, err)
	res, err := client.PresetsList(t.Context())
	require.NoError(t, err)
	page, ok := res.(*externalapi.PresetsListOK)
	require.True(t, ok, "%T", res)
	return lo.Map(page.Items, func(p externalapi.PresetResource, _ int) string { return p.Name })
}

func TestNewLoadsTheCustomPresetsOfTheInstanceDirectory(t *testing.T) {
	t.Setenv("SPHERICON_PRESETS_DIR", writePresetDir(t, map[string]string{"acme-relay.yaml": customPreset}))

	names := presetNames(t, newApp(t))

	assert.Contains(t, names, "acme-relay")
	assert.Contains(t, names, "generic", "the built-in Presets load beside the custom ones")
}

func TestNewStopsOnABrokenCustomPreset(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_PRESETS_DIR", writePresetDir(t, map[string]string{"typo.yaml": "version: 1\nname: [\n"}))

	_, err := app.New("test")

	require.ErrorContains(t, err, "typo.yaml")
}

func TestNewStopsOnACustomPresetThatReusesABuiltInName(t *testing.T) {
	baseline(t)
	mine := strings.Replace(customPreset, "acme-relay", "generic", 1)
	t.Setenv("SPHERICON_PRESETS_DIR", writePresetDir(t, map[string]string{"mine.yaml": mine}))

	_, err := app.New("test")

	require.ErrorContains(t, err, "generic")
}

func TestNewStopsWhenThePresetsDirectoryIsMissing(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_PRESETS_DIR", filepath.Join(t.TempDir(), "absent"))

	_, err := app.New("test")

	require.Error(t, err)
}
