package upgrade_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

// fakeSystemd puts `systemctl` and `systemd-run` on the PATH and returns the directory they
// keep their state in. Like the real ones they leave things behind rather than log calls:
//   - `systemctl show` reports the service's User and EnvironmentFiles;
//   - `systemctl restart` makes the service run whatever binary is at target, written to
//     <state>/running; `is-active` answers with the state in <state>/active (`active`/`failed`);
//   - `systemd-run` records the properties of the transient unit it creates in <state>/unit,
//     then runs the command it is given after its own options.
func fakeSystemd(t *testing.T, active, target string) (state string) {
	t.Helper()
	bin, state := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(state, "active"), []byte(active), 0o644))
	systemctl := `#!/bin/sh
state="` + state + `"
case "$1" in
show) printf 'User=sphericon\nEnvironmentFiles=/etc/sphericon/env (ignore_errors=no) /etc/sphericon/extra (ignore_errors=yes)\n' ;;
restart) cat "` + target + `" > "$state/running" ;;
is-active) cat "$state/active"; echo; [ "$(cat "$state/active")" = active ] ;;
esac
`
	run := `#!/bin/sh
state="` + state + `"
: > "$state/unit"
while [ $# -gt 0 ]; do
  case "$1" in
  -p) echo "$2" >> "$state/unit"; shift 2 ;;
  --*) shift ;;
  *) break ;;
  esac
done
exec "$@"
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "systemctl"), []byte(systemctl), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "systemd-run"), []byte(run), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return state
}

func (s setup) restartOpts(version string) upgrade.Options {
	o := s.opts(version)
	sd := upgrade.Systemd{Service: "sphericon", Out: s.out}
	o.Run, o.PostSwap = sd.Run, sd.PostSwap
	return o
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestRestartMigratesInATransientUnitWithTheServicesUserAndEnvironmentThenRestarts(t *testing.T) {
	s := newSetup(t)
	state := fakeSystemd(t, "active", s.target)
	release(t, s.dir, "0.2.0", "", false)

	_, err := upgrade.Step(t.Context(), s.restartOpts(""))

	require.NoError(t, err)
	assert.Equal(t, "ran\n", s.read(t, "migrated"))
	assert.Equal(t, "old binary", s.read(t, "sphericon.old"))
	assert.Equal(t, "User=sphericon\nEnvironmentFile=/etc/sphericon/env\nEnvironmentFile=-/etc/sphericon/extra\n", readFile(t, filepath.Join(state, "unit")))
	assert.Equal(t, s.read(t, "sphericon"), readFile(t, filepath.Join(state, "running")), "the service restarted into the new binary")
	assert.Contains(t, s.out.String(), "restarted sphericon")
	assert.NotContains(t, s.out.String(), "Restart to run it")
}

func TestRestartFailedMigrationLeavesTheOldBinaryServingAndNothingRestarted(t *testing.T) {
	s := newSetup(t)
	state := fakeSystemd(t, "active", s.target)
	release(t, s.dir, "0.2.0", "", true)

	_, err := upgrade.Step(t.Context(), s.restartOpts(""))

	require.Error(t, err)
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.NoFileExists(t, filepath.Join(state, "running"))
}

func TestRestartFailsWhenTheServiceDoesNotBecomeActive(t *testing.T) {
	s := newSetup(t)
	fakeSystemd(t, "failed", s.target)
	release(t, s.dir, "0.2.0", "", false)

	_, err := upgrade.Step(t.Context(), s.restartOpts(""))

	require.ErrorContains(t, err, "sphericon is failed")
	assert.FileExists(t, filepath.Join(s.install, "sphericon.old"))
}
