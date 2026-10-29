package upgrade_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

func TestCheckSucceedsWhenTheInstanceIsCurrent(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.2.0"})
	var out bytes.Buffer

	err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0", &out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "Up to date: 0.2.0")
}

func TestCheckSignalsANewerRelease(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.3.0"})
	var out bytes.Buffer

	err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0", &out)

	require.ErrorIs(t, err, upgrade.ErrNewerRelease)
	assert.Contains(t, out.String(), "A newer release is available: 0.3.0")
}

func TestCheckIsCurrentWhenTheInstanceIsAheadOfTheLatest(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.1.0"})

	err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0", &bytes.Buffer{})

	require.NoError(t, err)
}

func TestCheckFailsWhenGitHubDoes(t *testing.T) {
	gh := githubFake(t, t.TempDir())

	err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0", &bytes.Buffer{})

	require.Error(t, err)
	assert.NotErrorIs(t, err, upgrade.ErrNewerRelease)
}

// A check changes nothing, so it takes none of the options of an install.
func TestCheckRefusesTheOptionsOfAnInstall(t *testing.T) {
	for _, r := range []upgrade.Request{
		{Check: true, From: t.TempDir()},
		{Check: true, Version: "v0.2.0"},
		{Check: true, Yes: true},
		{Check: true, Restart: true},
	} {
		err := upgrade.Run(t.Context(), r)

		require.EqualError(t, err, upgrade.Usage, "%+v", r)
	}
}
