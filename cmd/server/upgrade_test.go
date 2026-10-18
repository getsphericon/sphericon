package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

func TestUpgradeRejectsUnknownFlags(t *testing.T) {
	err := execute(nil, &bytes.Buffer{}, "upgrade", "--bogus")

	assert.Error(t, err)
}

func TestUpgradeCheckRefusesADirectory(t *testing.T) {
	err := execute(nil, &bytes.Buffer{}, "upgrade", "--check", "--from", t.TempDir())

	assert.ErrorContains(t, err, "usage: upgrade")
}

func latestIs(t *testing.T, tag string, status int) *upgrade.GitHub {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
	}))
	t.Cleanup(srv.Close)
	gh, err := upgrade.NewGitHubAt(srv.URL)
	require.NoError(t, err)
	return gh
}

func TestCheckSucceedsWhenTheInstanceIsCurrent(t *testing.T) {
	out := &bytes.Buffer{}

	err := checkRelease(t.Context(), latestIs(t, "v0.2.0", http.StatusOK), "v0.2.0", out)

	require.NoError(t, err)
	assert.Contains(t, out.String(), "0.2.0")
}

func TestCheckSignalsANewerRelease(t *testing.T) {
	out := &bytes.Buffer{}

	err := checkRelease(t.Context(), latestIs(t, "v0.3.0", http.StatusOK), "v0.2.0", out)

	require.ErrorIs(t, err, errNewerRelease)
	assert.Contains(t, out.String(), "0.3.0")
}

func TestCheckIsCurrentWhenTheInstanceIsAheadOfTheLatest(t *testing.T) {
	err := checkRelease(t.Context(), latestIs(t, "v0.1.0", http.StatusOK), "v0.2.0", &bytes.Buffer{})

	require.NoError(t, err)
}

func TestCheckFailsWhenGitHubDoes(t *testing.T) {
	err := checkRelease(t.Context(), latestIs(t, "", http.StatusBadGateway), "v0.2.0", &bytes.Buffer{})

	require.Error(t, err)
	assert.NotErrorIs(t, err, errNewerRelease)
}
