// Package buildinfo names the build: version, commit and build time. The release build and the
// Dockerfile set them with -ldflags; a build without them (go build, go install, go run) reads
// the Go build info instead (ADR 0034).
package buildinfo

import (
	"fmt"
	"runtime/debug"
	"strings"

	"golang.org/x/mod/semver"
)

// Set by -ldflags "-X github.com/getsphericon/sphericon/internal/buildinfo.Version=..." etc.
var (
	Version string
	Commit  string
	Date    string
)

const shortCommit = 12

// Info is one build.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"built"`
}

// Current is the build of this binary.
func Current() Info {
	bi, _ := debug.ReadBuildInfo()
	return resolve(Info{Version: Version, Commit: Commit, Date: Date}, bi)
}

// Line is the one-line form `sphericon version` prints.
func (i Info) Line() string {
	return fmt.Sprintf("sphericon %s (commit %s, built %s)", i.Version, i.Commit, i.Date)
}

// resolve fills what the linker flags left empty from the Go build info.
func resolve(set Info, bi *debug.BuildInfo) Info {
	out := set
	var revision, vcsTime, module string
	if bi != nil {
		module = bi.Main.Version
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value
			case "vcs.time":
				vcsTime = s.Value
			}
		}
	}
	if out.Version == "" {
		out.Version = "dev"
		if module != "" && module != "(devel)" {
			out.Version = module
		}
	}
	if out.Commit == "" {
		out.Commit = "unknown"
		if revision != "" {
			out.Commit = revision[:min(len(revision), shortCommit)]
		}
	}
	if out.Date == "" {
		out.Date = "unknown"
		if vcsTime != "" {
			out.Date = vcsTime
		}
	}
	return out
}

// Semver is v in the "v1.2.3" form golang.org/x/mod/semver wants, whether it came with the
// leading "v" or without ("0.1.0", the form release archives and `--version` use).
func Semver(v string) string { return "v" + strings.TrimPrefix(v, "v") }

// Bare is v without the leading "v" ("0.1.0"), the form of archive names and Results.
func Bare(v string) string { return strings.TrimPrefix(v, "v") }

// IsRelease reports whether v is a stable release version ("v1.2.3", "1.2.3"): a plain
// semantic version, not "dev", a commit-derived pseudo-version, a prerelease or one with build
// metadata. It is the one answer to "is this a release" for the Telemetry report and the
// Release check (ADR 0032).
func IsRelease(v string) bool {
	s := Semver(v)
	return semver.IsValid(s) && semver.Prerelease(s) == "" && semver.Build(s) == ""
}
