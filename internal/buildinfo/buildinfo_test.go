package buildinfo

import (
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveKeepsWhatTheReleaseBuildSet(t *testing.T) {
	got := resolve(Info{Version: "v0.4.2", Commit: "abc1234", Date: "2026-10-18T10:00:00Z"}, &debug.BuildInfo{
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "ffffffffffff"}},
	})

	assert.Equal(t, Info{Version: "v0.4.2", Commit: "abc1234", Date: "2026-10-18T10:00:00Z"}, got)
}

func TestResolveFallsBackToGoBuildInfo(t *testing.T) {
	got := resolve(Info{}, &debug.BuildInfo{
		Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "0123456789abcdef0123"},
			{Key: "vcs.time", Value: "2026-10-18T09:00:00Z"},
		},
	})

	assert.Equal(t, Info{Version: "dev", Commit: "0123456789ab", Date: "2026-10-18T09:00:00Z"}, got)
}

func TestResolveUsesTheModuleVersionOfAGoInstall(t *testing.T) {
	got := resolve(Info{}, &debug.BuildInfo{Main: debug.Module{Version: "v0.5.0"}})

	assert.Equal(t, Info{Version: "v0.5.0", Commit: "unknown", Date: "unknown"}, got)
}

func TestResolveWithoutAnyBuildInfo(t *testing.T) {
	assert.Equal(t, Info{Version: "dev", Commit: "unknown", Date: "unknown"}, resolve(Info{}, nil))
}

func TestLine(t *testing.T) {
	line := Info{Version: "v0.4.2", Commit: "abc1234", Date: "2026-10-18T10:00:00Z"}.Line()

	assert.Equal(t, "sphericon v0.4.2 (commit abc1234, built 2026-10-18T10:00:00Z)", line)
}
