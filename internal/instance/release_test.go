package instance_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type fixedSource struct {
	version string
	err     error
}

func (s fixedSource) LatestRelease(context.Context) (string, error) { return s.version, s.err }

func logged(t *testing.T, running string, src instance.ReleaseSource) (*instance.Module, *bytes.Buffer) {
	t.Helper()
	env := testhelper.Setup(t)
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mod := instance.New(env.DB, instance.Environment{ReleaseCheck: true}, instance.Host{AppVersion: running}, nil,
		instance.WithReleaseSource(src), instance.WithLogger(log))
	return mod, buf
}

func TestFindingANewerReleaseIsLoggedOnceNamingTheReleaseAndUpgradeCheck(t *testing.T) {
	mod, buf := logged(t, "0.1.0", fixedSource{version: "0.2.0"})

	require.NoError(t, mod.CheckRelease(t.Context()))
	require.NoError(t, mod.CheckRelease(t.Context()))

	out := buf.String()
	assert.Equal(t, 1, bytes.Count(buf.Bytes(), []byte("a newer release is available")), out)
	assert.Contains(t, out, "latest=0.2.0")
	assert.Contains(t, out, "sphericon upgrade --check")
}

func TestStartLogsTheCachedNewerReleaseAndStaysQuietWithoutOne(t *testing.T) {
	mod, buf := logged(t, "0.1.0", fixedSource{version: "0.2.0"})
	require.NoError(t, mod.AnnounceRelease(t.Context()))
	assert.NotContains(t, buf.String(), "a newer release is available")

	require.NoError(t, mod.CheckRelease(t.Context()))
	buf.Reset()
	require.NoError(t, mod.AnnounceRelease(t.Context()))

	assert.Contains(t, buf.String(), "latest=0.2.0")
	assert.Contains(t, buf.String(), "sphericon upgrade --check")
}

func TestAnUnreachableSourceIsOnlyALogLineAtDebug(t *testing.T) {
	mod, buf := logged(t, "0.1.0", fixedSource{err: errors.New("connection refused")})

	require.NoError(t, mod.CheckRelease(t.Context()))

	assert.Contains(t, buf.String(), "level=DEBUG")
	assert.NotContains(t, buf.String(), "level=ERROR")
	assert.NotContains(t, buf.String(), "level=WARN")
}

func TestLatestReleaseReadsTheCachedResultWithoutAskingTheSource(t *testing.T) {
	mod, _ := logged(t, "0.1.0", fixedSource{version: "0.2.0"})
	empty, err := mod.LatestRelease(t.Context())
	require.NoError(t, err)
	assert.Empty(t, empty)

	require.NoError(t, mod.CheckRelease(t.Context()))
	latest, err := mod.LatestRelease(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "0.2.0", latest)
}
