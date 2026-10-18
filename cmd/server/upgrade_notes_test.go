package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtureChangelog(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("testdata/CHANGELOG.md")
	require.NoError(t, err)
	return string(b)
}

// notes runs `sphericon upgrade-notes <args>` on the fixture changelog.
func notes(t *testing.T, out *bytes.Buffer, args ...string) error {
	t.Helper()
	return runDeps(deps{changelog: fixtureChangelog(t)}, out, append([]string{"upgrade-notes"}, args...)...)
}

func TestUpgradeNotesPrintsTheEntriesOfEveryReleaseAfterTheVersion(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, notes(t, &out, "--since", "v1.0.0"))

	assert.Equal(t, "2.0.0\n  - export your segments, then install 2.0.0.\n\n1.1.0\n  - set SPHERICON_FOO before restarting\n", out.String())
}

func TestUpgradeNotesPrintsNothingWhenThereAreNone(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, notes(t, &out, "--since", "2.0.0"))

	assert.Empty(t, out.String())
}

func TestUpgradeNotesNeedsSince(t *testing.T) {
	assert.Error(t, notes(t, &bytes.Buffer{}))
	assert.Error(t, notes(t, &bytes.Buffer{}, "--since", "latest"))
}
