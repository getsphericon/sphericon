package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/preset"
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

func TestNewLoadsTheCustomPresetsOfTheInstanceDirectory(t *testing.T) {
	t.Setenv("SPHERICON_PRESETS_DIR", writePresetDir(t, map[string]string{"acme-relay.yaml": customPreset}))

	a := newApp(t)

	presets, err := do.Invoke[*preset.Registry](a.injector)
	require.NoError(t, err)
	assert.Contains(t, presets.Names(), "acme-relay")
	assert.Contains(t, presets.Names(), "generic", "the built-in Presets load beside the custom ones")
}

func TestNewStopsOnABrokenCustomPreset(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_PRESETS_DIR", writePresetDir(t, map[string]string{"typo.yaml": "version: 1\nname: [\n"}))

	_, err := New("test")

	require.ErrorContains(t, err, "typo.yaml")
}

func TestNewStopsOnACustomPresetThatReusesABuiltInName(t *testing.T) {
	baseline(t)
	mine := strings.Replace(customPreset, "acme-relay", "generic", 1)
	t.Setenv("SPHERICON_PRESETS_DIR", writePresetDir(t, map[string]string{"mine.yaml": mine}))

	_, err := New("test")

	require.ErrorContains(t, err, "generic")
}

func TestNewStopsWhenThePresetsDirectoryIsMissing(t *testing.T) {
	baseline(t)
	t.Setenv("SPHERICON_PRESETS_DIR", filepath.Join(t.TempDir(), "absent"))

	_, err := New("test")

	require.Error(t, err)
}
