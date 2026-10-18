// Package buildinfo names the build: version, commit and build time. The release build and the
// Dockerfile set them with -ldflags; a build without them (go build, go install, go run) reads
// the Go build info instead (ADR 0034).
package buildinfo

import (
	"fmt"
	"runtime/debug"
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
