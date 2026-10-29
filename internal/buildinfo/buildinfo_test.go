package buildinfo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/buildinfo"
)

// setLinkerFlags sets the variables the release build sets with -ldflags, for this test only.
func setLinkerFlags(t *testing.T, version, commit, date string) {
	t.Helper()
	prevVersion, prevCommit, prevDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit, buildinfo.Date = prevVersion, prevCommit, prevDate })
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = version, commit, date
}

// What the release build set wins over whatever the Go build info says.
func TestCurrentKeepsWhatTheReleaseBuildSet(t *testing.T) {
	setLinkerFlags(t, "v0.4.2", "abc1234", "2026-10-18T10:00:00Z")

	assert.Equal(t, buildinfo.Info{Version: "v0.4.2", Commit: "abc1234", Date: "2026-10-18T10:00:00Z"}, buildinfo.Current())
}

func TestLine(t *testing.T) {
	line := buildinfo.Info{Version: "v0.4.2", Commit: "abc1234", Date: "2026-10-18T10:00:00Z"}.Line()

	assert.Equal(t, "sphericon v0.4.2 (commit abc1234, built 2026-10-18T10:00:00Z)", line)
}
