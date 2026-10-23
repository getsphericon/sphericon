package upgrade_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/upgrade"
)

// boundaries is what a fake release's `migrate --status --json` prints: the database at
// stateMajor and the binary's table of three Majors, floors v0.3.2 and v0.5.1.
func boundaries(stateMajor int) string {
	return fmt.Sprintf(`{"state":{"major":%d,"floor":"v0.0.0"},"majors":[`+
		`{"major":1,"floor":"v0.0.0"},{"major":2,"floor":"v0.3.2"},{"major":3,"floor":"v0.5.1"}]}`, stateMajor)
}

// walkRelease writes a release whose binary appends its version to `installed` next to it on
// `migrate`, and prints status on `migrate --status --json`.
func walkRelease(t *testing.T, dir, version, status string) {
	t.Helper()
	walkReleaseWithNotes(t, dir, version, status, "")
}

// walkReleaseWithNotes is walkRelease whose `upgrade-notes` prints notes.
func walkReleaseWithNotes(t *testing.T, dir, version, status, notes string) {
	t.Helper()
	script := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n"+
		"'migrate --status --json') printf '%%s' '%s' ;;\n"+
		"upgrade-notes*) printf '%%s' '%s' ;;\n"+
		"migrate) echo %s >> \"$(dirname \"$0\")/installed\" ;;\n"+
		"esac\n", status, notes, version)
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

func (s setup) installed(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(s.install, "installed"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Fields(string(b))
}

func versions(rs []upgrade.Result) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Version)
	}
	return out
}

func TestWalkInstallsTheLastReleaseOfEachMajorThenTheTarget(t *testing.T) {
	s := newSetup(t)
	for _, v := range []string{"0.3.2", "0.4.0", "0.5.1", "0.6.0"} { // 0.4.0 is no boundary
		walkRelease(t, s.dir, v, boundaries(1))
	}
	o := s.opts("")

	res, err := upgrade.Walk(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, []string{"0.3.2", "0.5.1", "0.6.0"}, versions(res))
	assert.Equal(t, []string{"0.3.2", "0.5.1", "0.6.0"}, s.installed(t))
}

func TestWalkSkipsMajorsTheDatabaseAlreadyPassed(t *testing.T) {
	s := newSetup(t)
	for _, v := range []string{"0.3.2", "0.5.1", "0.6.0"} {
		walkRelease(t, s.dir, v, boundaries(2))
	}
	o := s.opts("")
	o.Current = "v0.4.0"

	res, err := upgrade.Walk(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, []string{"0.5.1", "0.6.0"}, versions(res))
}

func TestWalkInsideAMajorIsOneStep(t *testing.T) {
	s := newSetup(t)
	for _, v := range []string{"0.5.1", "0.5.4", "0.6.0"} {
		walkRelease(t, s.dir, v, boundaries(3))
	}
	o := s.opts("")
	o.Current = "v0.5.2"

	res, err := upgrade.Walk(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, []string{"0.6.0"}, versions(res))
}

func TestWalkToAPinnedTargetStopsBeforeLaterBoundaries(t *testing.T) {
	s := newSetup(t)
	for _, v := range []string{"0.3.2", "0.4.0", "0.5.1"} {
		walkRelease(t, s.dir, v, boundaries(1))
	}
	o := s.opts("v0.4.0")

	res, err := upgrade.Walk(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, []string{"0.3.2", "0.4.0"}, versions(res))
}

func TestWalkWithAMissingArchiveRefusesBeforeInstallingAnything(t *testing.T) {
	s := newSetup(t)
	for _, v := range []string{"0.5.1", "0.6.0"} { // 0.3.2 is missing
		walkRelease(t, s.dir, v, boundaries(1))
	}
	o := s.opts("")

	_, err := upgrade.Walk(t.Context(), o)

	require.ErrorContains(t, err, "0.3.2")
	assert.Empty(t, s.installed(t))
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.False(t, s.exists("sphericon.old"))
}

// Every release of the walk has its Upgrade notes shown, and --yes asked for, before the first
// migration or swap: here the notes sit in the target's changelog, which covers both Majors.
func TestWalkShowsTheNotesOfEveryReleaseAndWaitsForYesBeforeChangingAnything(t *testing.T) {
	s := newSetup(t)
	notes := "0.3.2\n  - export your segments\n\n0.6.0\n  - set SPHERICON_FOO\n"
	for _, v := range []string{"0.3.2", "0.5.1"} {
		walkRelease(t, s.dir, v, boundaries(1))
	}
	walkReleaseWithNotes(t, s.dir, "0.6.0", boundaries(1), notes)

	_, err := upgrade.Walk(t.Context(), s.opts(""))

	require.ErrorIs(t, err, upgrade.ErrNeedsConfirmation)
	assert.Contains(t, s.out.String(), "export your segments")
	assert.Contains(t, s.out.String(), "set SPHERICON_FOO")
	assert.Empty(t, s.installed(t))
	assert.Equal(t, "old binary", s.read(t, "sphericon"))
	assert.False(t, s.exists("sphericon.old"))
	assert.False(t, s.exists("sphericon.new"))
	assert.False(t, s.exists("sphericon.plan"))

	o := s.opts("")
	o.Yes = true
	s.out.Reset()
	res, err := upgrade.Walk(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, []string{"0.3.2", "0.5.1", "0.6.0"}, versions(res))
	assert.Equal(t, 1, strings.Count(s.out.String(), "export your segments"), "the notes are shown once, not per step")
	assert.False(t, s.exists("sphericon.plan"))
}

// onceSource lets the target's archive be fetched one time only.
type onceSource struct {
	upgrade.Source

	target string
	used   bool
}

func (o *onceSource) Fetch(ctx context.Context, version string, p upgrade.Platform) (upgrade.Release, error) {
	if version == o.target {
		if o.used {
			return upgrade.Release{}, errors.New("the target was fetched twice")
		}
		o.used = true
	}
	return o.Source.Fetch(ctx, version, p)
}

func TestWalkInstallsTheTargetFromTheCopyItStagedInsteadOfFetchingItAgain(t *testing.T) {
	s := newSetup(t)
	for _, v := range []string{"0.3.2", "0.5.1", "0.6.0"} {
		walkRelease(t, s.dir, v, boundaries(1))
	}
	o := s.opts("")
	o.Source = &onceSource{Source: o.Source, target: "0.6.0"}

	res, err := upgrade.Walk(t.Context(), o)

	require.NoError(t, err)
	assert.Equal(t, []string{"0.3.2", "0.5.1", "0.6.0"}, versions(res))
	assert.Equal(t, []string{"0.3.2", "0.5.1", "0.6.0"}, s.installed(t))
}
