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

func TestUpgradeNotesPrintsTheEntriesOfEveryReleaseAfterTheVersion(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, runUpgradeNotes(&out, []string{"--since", "v1.0.0"}, fixtureChangelog(t)))

	assert.Equal(t, "2.0.0\n  - export your segments, then install 2.0.0.\n\n1.1.0\n  - set SPHERICON_FOO before restarting\n", out.String())
}

func TestUpgradeNotesPrintsNothingWhenThereAreNone(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, runUpgradeNotes(&out, []string{"--since", "2.0.0"}, fixtureChangelog(t)))

	assert.Empty(t, out.String())
}

func TestUpgradeNotesNeedsSince(t *testing.T) {
	assert.Error(t, runUpgradeNotes(&bytes.Buffer{}, nil, fixtureChangelog(t)))
	assert.Error(t, runUpgradeNotes(&bytes.Buffer{}, []string{"--since", "latest"}, fixtureChangelog(t)))
}
