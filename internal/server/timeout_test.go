package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/server"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// deadlineOf is how long a request to path may run under the timeouts of limits: what is left of
// its context's deadline when the handler starts.
func deadlineOf(t *testing.T, limits config.BodyLimits, path string) time.Duration {
	t.Helper()
	var (
		deadline time.Time
		ok       bool
	)
	h := server.Timeouts(limits, time.Now)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		deadline, ok = r.Context().Deadline()
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))
	require.True(t, ok, "every request has a deadline")
	return time.Until(deadline)
}

// The upload of a CSV import's file runs under a timeout that fits its own cap, not the 30 seconds
// of every other request: a 50 MiB file at a megabit a second takes seven minutes.
func TestTheUploadOfAnImportRunsUnderATimeoutFittingItsCap(t *testing.T) {
	limits := config.BodyLimits{Default: 1 << 20, CSVImport: 50 << 20}

	upload := deadlineOf(t, limits, "/site/workspaces/ws_1/contact-imports")
	other := deadlineOf(t, limits, "/site/workspaces/ws_1/contacts")

	assert.Greater(t, upload, 7*time.Minute)
	assert.Greater(t, upload-other, 6*time.Minute, "the cap's share beside the request timeout of the rest")
}

// deadlineRecorder is a response writer whose connection takes a read deadline, as a served
// request's does: it keeps the last one set.
type deadlineRecorder struct {
	*httptest.ResponseRecorder

	readDeadline time.Time
}

func (d *deadlineRecorder) SetReadDeadline(t time.Time) error {
	d.readDeadline = t
	return nil
}

// The server's ReadTimeout bounds the reading of a body too: through the whole server, the upload
// of an import's file moves its read deadline to the time its cap takes, by the Instance clock,
// and any other request leaves it.
func TestTheUploadOfAnImportMovesItsReadDeadline(t *testing.T) {
	at := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	for _, mode := range []testhelper.Option{production(), development()} {
		env := testhelper.Setup(t, mode, testhelper.WithClock(func() time.Time { return at }))
		post := func(path string) time.Time {
			rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
			env.Server.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil))
			return rec.readDeadline
		}

		upload := post("/site/workspaces/" + fixtures.AcmePublicID + "/contact-imports")
		other := post("/site/workspaces/" + fixtures.AcmePublicID + "/contacts")

		assert.Greater(t, upload.Sub(at), 7*time.Minute)
		assert.True(t, other.IsZero(), "another request keeps the server's ReadTimeout")
	}
}
