package upgrade_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
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
	script := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n"+
		"'migrate --status --json') printf '%%s' '%s' ;;\n"+
		"migrate) echo %s >> \"$(dirname \"$0\")/installed\" ;;\n"+
		"esac\n", status, version)
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
	var out []string
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
