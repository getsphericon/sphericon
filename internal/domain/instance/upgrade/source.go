package upgrade

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Platform is the OS and architecture a release archive is built for.
type Platform struct{ OS, Arch string }

// ArchiveName is the release archive for a version (no leading "v") and platform, as the
// release build names it.
func ArchiveName(version string, p Platform) string {
	return fmt.Sprintf("sphericon_%s_%s_%s.tar.gz", version, p.OS, p.Arch)
}

// Release is one release's archive and checksums.txt, staged on local disk.
type Release struct {
	Version       string // without the leading "v"
	ArchiveName   string
	ArchivePath   string
	ChecksumsPath string
	// Cleanup removes what the source staged; nil when nothing was (a directory source).
	Cleanup func()
}

// Source is where releases come from: a directory of downloaded files, or GitHub Releases.
// The Major walk asks it for versions and fetches one per step.
type Source interface {
	// Versions lists the releases it can supply for the platform (without the leading "v").
	Versions(ctx context.Context, p Platform) ([]string, error)
	// Fetch stages one release (version without the leading "v") for the platform.
	Fetch(ctx context.Context, version string, p Platform) (Release, error)
}

// DirSource reads archives and checksums.txt already on the machine (`upgrade --from <dir>`).
func DirSource(dir string) Source { return dirSource(dir) }

type dirSource string

func (d dirSource) Versions(_ context.Context, p Platform) ([]string, error) {
	suffix := fmt.Sprintf("_%s_%s.tar.gz", p.OS, p.Arch)
	entries, err := os.ReadDir(string(d))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if v, ok := strings.CutPrefix(name, "sphericon_"); ok {
			if v, ok = strings.CutSuffix(v, suffix); ok {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

func (d dirSource) Fetch(_ context.Context, version string, p Platform) (Release, error) {
	name := ArchiveName(version, p)
	r := Release{
		Version:       version,
		ArchiveName:   name,
		ArchivePath:   filepath.Join(string(d), name),
		ChecksumsPath: filepath.Join(string(d), "checksums.txt"),
	}
	for _, f := range []string{r.ArchivePath, r.ChecksumsPath} {
		if _, err := os.Stat(f); err != nil {
			return Release{}, fmt.Errorf("release %s: %w", version, err)
		}
	}
	return r, nil
}
