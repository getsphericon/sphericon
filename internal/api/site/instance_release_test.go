package site_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// releases is a GitHub Releases source whose answer a test changes: a tag, or a status that
// fails the request.
type releases struct {
	tag    atomic.Value
	status atomic.Int64
}

func newReleases(t *testing.T, tag string) (*releases, string) {
	t.Helper()
	r := &releases{}
	r.tag.Store(tag)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if s := int(r.status.Load()); s != 0 {
			http.Error(w, "down", s)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": r.tag.Load(), "assets": []any{}})
	}))
	t.Cleanup(srv.Close)
	return r, srv.URL
}

func releaseEnv(t *testing.T, version, url string, edit ...func(*config.Config)) (*testhelper.TestEnv, *time.Time) {
	t.Helper()
	now := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	env := testhelper.Setup(t,
		testhelper.WithRunningVersion(version),
		testhelper.WithClock(func() time.Time { return now }),
		testhelper.WithConfig(func(c *config.Config) {
			c.GitHubAPIURL = url
			c.ReleaseCheck = true
			for _, e := range edit {
				e(c)
			}
		}))
	return env, &now
}

func TestReleaseCheckStoresANewerReleaseAndTheStateShowsItToAnOwner(t *testing.T) {
	_, url := newReleases(t, "v0.2.0")
	env, now := releaseEnv(t, "0.1.0", url)

	require.NoError(t, env.Instance.CheckRelease(t.Context()))

	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, row.LatestRelease)
	assert.Equal(t, "0.2.0", *row.LatestRelease)
	require.NotNil(t, row.LatestReleaseCheckedAt)
	assert.True(t, row.LatestReleaseCheckedAt.Equal(*now))
	state := instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail))
	assert.Equal(t, "0.2.0", state.LatestRelease.Value)
	requireInstanceForbidden(t, env.SiteActor(t, fixtures.MemberMaryEmail))
}

func TestReleaseCheckNeverStoresAnEqualOlderOrPrerelease(t *testing.T) {
	for name, tag := range map[string]string{"equal": "v0.1.0", "older": "v0.0.9", "prerelease": "v0.3.0-rc.1"} {
		t.Run(name, func(t *testing.T) {
			_, url := newReleases(t, tag)
			env, _ := releaseEnv(t, "0.1.0", url)

			require.NoError(t, env.Instance.CheckRelease(t.Context()))

			row, err := env.Instance.Ensure(t.Context())
			require.NoError(t, err)
			assert.Nil(t, row.LatestRelease)
			assert.Nil(t, row.LatestReleaseCheckedAt)
			assert.False(t, instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).LatestRelease.Set)
		})
	}
}

func TestReleaseCheckKeepsThePreviousResultWhenTheSourceIsDown(t *testing.T) {
	src, url := newReleases(t, "v0.2.0")
	env, now := releaseEnv(t, "0.1.0", url)
	require.NoError(t, env.Instance.CheckRelease(t.Context()))
	first := *now

	*now = now.Add(24 * time.Hour)
	src.status.Store(http.StatusBadGateway)
	require.NoError(t, env.Instance.CheckRelease(t.Context()))

	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, row.LatestRelease)
	assert.Equal(t, "0.2.0", *row.LatestRelease)
	assert.True(t, row.LatestReleaseCheckedAt.Equal(first))
	assert.Equal(t, "0.2.0", instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).LatestRelease.Value)
}

func TestReleaseCheckSwitchedOffAsksNobodyAndShowsNothing(t *testing.T) {
	var asked atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.2.0", "assets": []any{}})
	}))
	t.Cleanup(srv.Close)
	env, _ := releaseEnv(t, "0.1.0", srv.URL, func(c *config.Config) {
		c.ReleaseCheck = false
		on := true
		c.Telemetry = &on
	})

	require.NoError(t, env.Instance.CheckRelease(t.Context()))

	row, err := env.Instance.Ensure(t.Context())
	require.NoError(t, err)
	assert.Nil(t, row.LatestRelease)
	assert.False(t, asked.Load())
}

func TestReleaseCheckIgnoresTheTelemetrySwitches(t *testing.T) {
	_, url := newReleases(t, "v0.2.0")
	env, _ := releaseEnv(t, "0.1.0", url, func(c *config.Config) {
		off := false
		c.Telemetry = &off
		c.DoNotTrack = true
	})

	require.NoError(t, env.Instance.CheckRelease(t.Context()))

	state := instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail))
	assert.Equal(t, "0.2.0", state.LatestRelease.Value)
	assert.False(t, state.Telemetry.Enabled)
}

func TestAReleaseThatIsNoLongerNewerIsNotShownAfterAnUpgrade(t *testing.T) {
	env, _ := releaseEnv(t, "0.2.0", "")
	require.NoError(t, env.DB.InstanceState.Create().SetID(1).SetLatestRelease("0.2.0").Exec(t.Context()))

	assert.False(t, instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).LatestRelease.Set)
}

func TestABuildThatIsNotAReleaseNeitherAsksNorShowsABanner(t *testing.T) {
	var asked atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.0.0", "assets": []any{}})
	}))
	t.Cleanup(srv.Close)
	env, _ := releaseEnv(t, "dev", srv.URL)

	require.NoError(t, env.Instance.CheckRelease(t.Context()))

	row, err := env.Instance.Ensure(t.Context())
	require.NoError(t, err)
	assert.Nil(t, row.LatestRelease)
	assert.False(t, asked.Load())
	assert.False(t, instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail)).LatestRelease.Set)
}
