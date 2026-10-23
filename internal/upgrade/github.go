package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v88/github"
	"golang.org/x/mod/semver"
	"golang.org/x/net/http/httpproxy"

	"github.com/getsphericon/sphericon/internal/buildinfo"
)

const (
	githubOwner = "getsphericon"
	githubRepo  = "sphericon"
)

// GitHub reads the public GitHub Releases of the project. It is a Source for `upgrade`, and its
// LatestRelease is the one question "is there a newer release" that the Release check
// (ADR 0032) and `upgrade --check` share.
type GitHub struct {
	client *github.Client
	http   *http.Client
}

// NewGitHub points at api.github.com. It honours HTTP_PROXY, HTTPS_PROXY and NO_PROXY.
func NewGitHub() (*GitHub, error) { return NewGitHubAt("") }

// NewGitHubAt points at another API root (a test server, GitHub Enterprise); "" is api.github.com.
func NewGitHubAt(baseURL string) (*GitHub, error) {
	def, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("http.DefaultTransport is a %T, not an *http.Transport", http.DefaultTransport)
	}
	t := def.Clone()
	// Read now, not through http.ProxyFromEnvironment, which caches the first read per process.
	proxy := httpproxy.FromEnvironment().ProxyFunc()
	t.Proxy = func(r *http.Request) (*url.URL, error) { return proxy(r.URL) }
	hc := &http.Client{Transport: t, Timeout: 10 * time.Minute}
	opts := []github.ClientOptionsFunc{github.WithHTTPClient(hc), github.WithUserAgent("sphericon-upgrade")}
	if baseURL != "" {
		base := strings.TrimSuffix(baseURL, "/") + "/"
		opts = append(opts, github.WithURLs(&base, nil))
	}
	c, err := github.NewClient(opts...)
	if err != nil {
		return nil, err
	}
	return &GitHub{client: c, http: hc}, nil
}

// LatestRelease is the newest stable release (GitHub's `latest`: no prerelease, no draft),
// without the leading "v".
func (g *GitHub) LatestRelease(ctx context.Context) (string, error) {
	r, _, err := g.client.Repositories.GetLatestRelease(ctx, githubOwner, githubRepo)
	if err != nil {
		return "", err
	}
	v := buildinfo.Bare(r.GetTagName())
	if !semver.IsValid(buildinfo.Semver(v)) {
		return "", fmt.Errorf("the latest release is tagged %q, which is not a version", r.GetTagName())
	}
	return v, nil
}

// Versions lists the stable releases that carry an archive for the platform.
func (g *GitHub) Versions(ctx context.Context, p Platform) ([]string, error) {
	rs, _, err := g.client.Repositories.ListReleases(ctx, githubOwner, githubRepo, &github.ListOptions{PerPage: 100})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rs {
		v := buildinfo.Bare(r.GetTagName())
		if r.GetDraft() || r.GetPrerelease() || !semver.IsValid(buildinfo.Semver(v)) {
			continue
		}
		if asset(r, ArchiveName(v, p)) != nil {
			out = append(out, v)
		}
	}
	return out, nil
}

// Fetch downloads the archive and checksums.txt of a release into a temporary directory.
func (g *GitHub) Fetch(ctx context.Context, version string, p Platform) (Release, error) {
	r, _, err := g.client.Repositories.GetReleaseByTag(ctx, githubOwner, githubRepo, buildinfo.Semver(version))
	if err != nil {
		return Release{}, fmt.Errorf("release %s: %w", version, err)
	}
	name := ArchiveName(version, p)
	dir, err := os.MkdirTemp("", "sphericon-upgrade-")
	if err != nil {
		return Release{}, err
	}
	rel := Release{
		Version:       version,
		ArchiveName:   name,
		ArchivePath:   filepath.Join(dir, name),
		ChecksumsPath: filepath.Join(dir, "checksums.txt"),
		Cleanup:       func() { _ = os.RemoveAll(dir) },
	}
	for _, f := range []struct{ asset, path string }{{name, rel.ArchivePath}, {"checksums.txt", rel.ChecksumsPath}} {
		a := asset(r, f.asset)
		if a == nil {
			rel.Cleanup()
			return Release{}, fmt.Errorf("release %s has no %s", version, f.asset)
		}
		if err := g.download(ctx, a.GetID(), f.path); err != nil {
			rel.Cleanup()
			return Release{}, fmt.Errorf("download %s: %w", f.asset, err)
		}
	}
	return rel, nil
}

func asset(r *github.RepositoryRelease, name string) *github.ReleaseAsset {
	for _, a := range r.Assets {
		if a.GetName() == name {
			return a
		}
	}
	return nil
}

func (g *GitHub) download(ctx context.Context, id int64, dst string) error {
	rc, _, err := g.client.Repositories.DownloadReleaseAsset(ctx, githubOwner, githubRepo, id, g.http)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, rc)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
