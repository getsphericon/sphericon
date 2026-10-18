package upgrade_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

// fakeSystemd puts `systemctl` and `systemd-run` on the PATH. They log every call to
// calls.log; `systemctl show` reports the service's User and EnvironmentFiles, and
// `systemctl is-active` answers with `active`. systemd-run runs the command it is given
// after its own options, like the real one.
func fakeSystemd(t *testing.T, active string) (log string) {
	t.Helper()
	bin := t.TempDir()
	log = filepath.Join(bin, "calls.log")
	systemctl := `#!/bin/sh
echo "systemctl $*" >> "` + log + `"
case "$1" in
show) printf 'User=sphericon\nEnvironmentFiles=/etc/sphericon/env (ignore_errors=no) /etc/sphericon/extra (ignore_errors=yes)\n' ;;
is-active) echo ` + active + `; [ ` + active + ` = active ] ;;
esac
`
	run := `#!/bin/sh
echo "systemd-run $*" >> "` + log + `"
while [ $# -gt 0 ]; do
  case "$1" in
  -p) shift 2 ;;
  --*) shift ;;
  *) break ;;
  esac
done
exec "$@"
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "systemctl"), []byte(systemctl), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "systemd-run"), []byte(run), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func (s setup) restartOpts(version string) upgrade.Options {
	o := s.opts(version)
	sd := upgrade.Systemd{Service: "sphericon", Out: s.out}
	o.Run, o.PostSwap = sd.Run, sd.PostSwap
	return o
}

func calls(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	require.NoError(t, err)
	return string(b)
}

func TestRestartMigratesInATransientUnitWithTheServicesUserAndEnvironmentThenRestarts(t *testing.T) {
	log := fakeSystemd(t, "active")
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)

	_, err := upgrade.Step(t.Context(), s.restartOpts(""))

	require.NoError(t, err)
	assert.Equal(t, "ran\n", s.read(t, "migrated"))
	assert.Equal(t, "old binary", s.read(t, "sphericon.old"))
	c := calls(t, log)
	assert.Contains(t, c, "systemd-run --wait --collect --pipe -p User=sphericon -p EnvironmentFile=/etc/sphericon/env -p EnvironmentFile=-/etc/sphericon/extra ")
	assert.Contains(t, c, "systemctl restart sphericon\n")
	assert.Contains(t, c, "systemctl is-active sphericon\n")
	assert.NotContains(t, s.out.String(), "Restart to run it")
}

func TestRestartFailedMigrationLeavesTheOldBinaryServingAndNothingRestarted(t *testing.T) {
	log := fakeSystemd(t, "active")
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", true)

	_, err := upgrade.Step(t.Context(), s.restartOpts(""))

	require.Error(t, err)
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.NotContains(t, calls(t, log), "restart")
}

func TestRestartFailsWhenTheServiceDoesNotBecomeActive(t *testing.T) {
	fakeSystemd(t, "failed")
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)

	_, err := upgrade.Step(t.Context(), s.restartOpts(""))

	assert.ErrorContains(t, err, "sphericon is failed")
	assert.FileExists(t, filepath.Join(s.install, "sphericon.old"))
}
