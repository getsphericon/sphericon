package upgrade_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/instance/upgrade"
)

func TestCheckSucceedsWhenTheInstanceIsCurrent(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.2.0"})
	latest, err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0")

	require.NoError(t, err)
	assert.Equal(t, upgrade.Latest{Release: "0.2.0", Running: "v0.2.0"}, latest)
}

func TestCheckSignalsANewerRelease(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.3.0"})
	latest, err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0")

	require.NoError(t, err)
	assert.Equal(t, upgrade.Latest{Release: "0.3.0", Running: "v0.2.0", Newer: true}, latest)
}

func TestCheckIsCurrentWhenTheInstanceIsAheadOfTheLatest(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.1.0"})

	latest, err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0")

	require.NoError(t, err)
	assert.False(t, latest.Newer)
}

func TestCheckFailsWhenGitHubDoes(t *testing.T) {
	gh := githubFake(t, t.TempDir())

	_, err := upgrade.CheckLatest(t.Context(), gh, "v0.2.0")

	require.Error(t, err)
}

// A check changes nothing, so it takes none of the options of an install.
func TestCheckRefusesTheOptionsOfAnInstall(t *testing.T) {
	for _, r := range []upgrade.Request{
		{Check: true, From: t.TempDir()},
		{Check: true, Version: "v0.2.0"},
		{Check: true, Yes: true},
		{Check: true, Restart: true},
	} {
		_, err := drain(upgrade.Run(t.Context(), r))

		require.EqualError(t, err, upgrade.Usage, "%+v", r)
	}
}
