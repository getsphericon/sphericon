package upgrade_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

var linuxArm = upgrade.Platform{OS: "linux", Arch: "arm64"}

// release writes sphericon_<version>_linux_arm64.tar.gz whose binary is a script: `migrate`
// drops a marker next to it (or fails), `upgrade-notes` prints notes.
func release(t *testing.T, dir, version, notes string, failMigrate bool) {
	t.Helper()
	migrate := `echo ran > "$(dirname "$0")/migrated"`
	if failMigrate {
		migrate = `echo boom >&2; exit 3`
	}
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\nmigrate) %s ;;\nupgrade-notes) printf '%%s' '%s' ;;\nesac\n", migrate, notes)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "sphericon", Mode: 0o755, Size: int64(len(script))}))
	_, err := tw.Write([]byte(script))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	name := fmt.Sprintf("sphericon_%s_linux_arm64.tar.gz", version)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644))
	sum := sha256.Sum256(buf.Bytes())
	f, err := os.OpenFile(filepath.Join(dir, "checksums.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = fmt.Fprintf(f, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

type setup struct {
	dir, install, target string
	out                  *bytes.Buffer
}

func newSetup(t *testing.T) setup {
	t.Helper()
	install := t.TempDir()
	target := filepath.Join(install, "sphericon")
	require.NoError(t, os.WriteFile(target, []byte("old binary"), 0o755))
	return setup{dir: t.TempDir(), install: install, target: target, out: &bytes.Buffer{}}
}

func (s setup) opts(version string) upgrade.Options {
	return upgrade.Options{
		Source: upgrade.DirSource(s.dir), Target: s.target, Current: "v0.1.0",
		Version: version, Platform: linuxArm, Out: s.out,
		Container: func() string { return "" },
	}
}

func (s setup) read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.install, name))
	require.NoError(t, err)
	return string(b)
}

func (s setup) exists(name string) bool {
	_, err := os.Stat(filepath.Join(s.install, name))
	return err == nil
}

func TestUpgradeMigratesWithTheNewBinarySwapsAndKeepsTheOldOne(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)

	res, err := upgrade.Step(t.Context(), s.opts(""))

	require.NoError(t, err)
	assert.Equal(t, "0.2.0", res.Version)
	assert.Equal(t, "old binary", s.read(t, "sphericon.old"))
	assert.Contains(t, s.read(t, "sphericon"), "upgrade-notes")
	assert.True(t, s.exists("migrated"))
	assert.False(t, s.exists("sphericon.new"))
	assert.Contains(t, s.out.String(), "systemctl restart sphericon")
}

func TestUpgradeWithAChecksumMismatchChangesNothing(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)
	require.NoError(t, os.WriteFile(filepath.Join(s.dir, "checksums.txt"),
		[]byte(fmt.Sprintf("%064d  sphericon_0.2.0_linux_arm64.tar.gz\n", 0)), 0o644))

	_, err := upgrade.Step(t.Context(), s.opts(""))

	require.ErrorContains(t, err, "checksum")
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.False(t, s.exists("migrated"))
	assert.False(t, s.exists("sphericon.old"))
	assert.False(t, s.exists("sphericon.new"))
}

func TestUpgradeStopsBeforeTheSwapWhenTheMigrationFails(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", true)

	_, err := upgrade.Step(t.Context(), s.opts(""))

	require.Error(t, err)
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.False(t, s.exists("sphericon.old"))
	assert.False(t, s.exists("sphericon.new"))
}

func TestUpgradeWaitsForYesWhenThereAreNotes(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "0.2.0\n  - export your segments\n", false)

	_, err := upgrade.Step(t.Context(), s.opts(""))

	require.ErrorIs(t, err, upgrade.ErrNeedsConfirmation)
	assert.Contains(t, s.out.String(), "export your segments")
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.False(t, s.exists("migrated"))

	o := s.opts("")
	o.Yes = true
	_, err = upgrade.Step(t.Context(), o)

	require.NoError(t, err)
	assert.True(t, s.exists("migrated"))
	assert.Equal(t, "old binary", s.read(t, "sphericon.old"))
}

func TestUpgradeRefusesInAContainerAndNamesTheCommand(t *testing.T) {
	for kind, want := range map[string]string{"docker": "docker", "helm": "helm upgrade"} {
		s := newSetup(t)
		release(t, s.dir, "0.2.0", "", false)
		o := s.opts("")
		o.Container = func() string { return kind }

		_, err := upgrade.Step(t.Context(), o)

		require.ErrorContains(t, err, want)
		assert.Equal(t, "old binary", s.read(t, "sphericon"))
	}
}

func TestUpgradeRefusesADowngradeAndLeavesACurrentInstanceAlone(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.0.9", "", false)
	_, err := upgrade.Step(t.Context(), s.opts("v0.0.9"))
	require.ErrorIs(t, err, upgrade.ErrDowngrade)

	s2 := newSetup(t)
	release(t, s2.dir, "0.1.0", "", false)
	res, err := upgrade.Step(t.Context(), s2.opts(""))
	require.NoError(t, err)
	assert.False(t, res.Changed)
	assert.Equal(t, "old binary", s2.read(t, "sphericon"))
}

func TestUpgradeTakesTheNewestReleaseInTheDirectoryUnlessOnePinned(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)
	release(t, s.dir, "0.10.0", "", false)

	pinned, err := upgrade.Step(t.Context(), s.opts("v0.2.0"))
	require.NoError(t, err)
	assert.Equal(t, "0.2.0", pinned.Version)

	s2 := newSetup(t)
	release(t, s2.dir, "0.2.0", "", false)
	release(t, s2.dir, "0.10.0", "", false)
	newest, err := upgrade.Step(t.Context(), s2.opts(""))
	require.NoError(t, err)
	assert.Equal(t, "0.10.0", newest.Version)
}

func TestUpgradeNeedsAnArchiveForThisPlatform(t *testing.T) {
	s := newSetup(t)

	_, err := upgrade.Step(t.Context(), s.opts("v0.2.0"))

	require.Error(t, err)
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
}

func TestPostSwapStepReplacesTheRestartHint(t *testing.T) {
	s := newSetup(t)
	release(t, s.dir, "0.2.0", "", false)
	var got upgrade.Result
	o := s.opts("")
	o.PostSwap = func(_ context.Context, r upgrade.Result) error { got = r; return nil }

	_, err := upgrade.Step(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, "0.2.0", got.Version)
	assert.NotContains(t, s.out.String(), "systemctl")
}
