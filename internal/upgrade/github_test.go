package upgrade_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

const repo = "getsphericon/sphericon"

type fakeRelease struct {
	Tag        string
	Prerelease bool
}

// githubFake serves the GitHub Releases API for repo; each release's files are the ones
// `release` wrote into dir.
func githubFake(t *testing.T, dir string, releases ...fakeRelease) *upgrade.GitHub {
	t.Helper()
	mux := http.NewServeMux()
	body := func(r fakeRelease) map[string]any {
		assets := make([]map[string]any, 0, 2)
		for _, name := range []string{"sphericon_" + strings.TrimPrefix(r.Tag, "v") + "_linux_arm64.tar.gz", "checksums.txt"} {
			assets = append(assets, map[string]any{"name": name, "id": assetID(name)})
		}
		return map[string]any{"tag_name": r.Tag, "prerelease": r.Prerelease, "assets": assets}
	}
	mux.HandleFunc("GET /repos/"+repo+"/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		for _, r := range releases {
			if !r.Prerelease {
				assert.NoError(t, json.NewEncoder(w).Encode(body(r)))
				return
			}
		}
		http.NotFound(w, nil)
	})
	mux.HandleFunc("GET /repos/"+repo+"/releases/tags/{tag}", func(w http.ResponseWriter, req *http.Request) {
		for _, r := range releases {
			if r.Tag == req.PathValue("tag") {
				assert.NoError(t, json.NewEncoder(w).Encode(body(r)))
				return
			}
		}
		http.NotFound(w, req)
	})
	mux.HandleFunc("GET /repos/"+repo+"/releases", func(w http.ResponseWriter, _ *http.Request) {
		all := make([]map[string]any, 0, len(releases))
		for _, r := range releases {
			all = append(all, body(r))
		}
		assert.NoError(t, json.NewEncoder(w).Encode(all))
	})
	mux.HandleFunc("GET /repos/"+repo+"/releases/assets/{id}", func(w http.ResponseWriter, req *http.Request) {
		for _, f := range files(t, dir) {
			if strconv.FormatInt(assetID(f), 10) == req.PathValue("id") {
				http.ServeFile(w, req, filepath.Join(dir, f))
				return
			}
		}
		http.NotFound(w, req)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	gh, err := upgrade.NewGitHubAt(srv.URL)
	require.NoError(t, err)
	return gh
}

// assetID derives a stable id from a file name.
func assetID(name string) int64 {
	var id int64 = 1
	for _, c := range name {
		id = id*31 + int64(c)
		id %= 1 << 40
	}
	return id
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestLatestReleaseIsTheNewestStableOne(t *testing.T) {
	gh := githubFake(t, t.TempDir(), fakeRelease{Tag: "v0.3.0-rc.1", Prerelease: true}, fakeRelease{Tag: "v0.2.0"}, fakeRelease{Tag: "v0.1.0"})

	v, err := gh.LatestRelease(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "0.2.0", v)
}

func TestLatestReleaseFailsWhenGitHubDoes(t *testing.T) {
	gh := githubFake(t, t.TempDir())

	_, err := gh.LatestRelease(t.Context())

	assert.Error(t, err)
}

func TestUpgradeDownloadsTheLatestRelease(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)
	gh := githubFake(t, s.dir, fakeRelease{Tag: "v0.2.0"})
	o := s.opts("")
	o.Source = gh

	res, err := upgrade.Step(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, "0.2.0", res.Version)
	assert.True(t, res.Changed)
	assert.Equal(t, "old binary", s.read(t, "sphericon.old"))
	assert.True(t, s.exists("migrated"))
}

func TestUpgradeDownloadsThePinnedRelease(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)
	release(t, s.dir, "0.3.0", "", false)
	gh := githubFake(t, s.dir, fakeRelease{Tag: "v0.3.0"}, fakeRelease{Tag: "v0.2.0"})
	o := s.opts("v0.2.0")
	o.Source = gh

	res, err := upgrade.Step(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, "0.2.0", res.Version)
}

func TestUpgradeRefusesADowngradeFromGitHub(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.0.9", "", false)
	gh := githubFake(t, s.dir, fakeRelease{Tag: "v0.0.9"})
	o := s.opts("v0.0.9")
	o.Source = gh

	_, err := upgrade.Step(t.Context(), o)

	assert.ErrorIs(t, err, upgrade.ErrDowngrade)
}

func TestUpgradeRejectsATamperedDownload(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "sphericon_0.2.0_linux_arm64.tar.gz"), []byte("tampered"), 0o644))
	gh := githubFake(t, s.dir, fakeRelease{Tag: "v0.2.0"})
	o := s.opts("")
	o.Source = gh

	_, err := upgrade.Step(t.Context(), o)

	require.ErrorContains(t, err, "checksum mismatch")
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
}

func TestGitHubHonoursTheProxyVariables(t *testing.T) {
	hit := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- r.URL.Path
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"tag_name": "v9.9.9"}))
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	// httptest listens on loopback, which ProxyFromEnvironment never proxies; a named host does.
	remote, err := upgrade.NewGitHubAt("http://github.test")
	require.NoError(t, err)

	v, err := remote.LatestRelease(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "9.9.9", v)
	assert.Equal(t, "/repos/"+repo+"/releases/latest", <-hit)
}
