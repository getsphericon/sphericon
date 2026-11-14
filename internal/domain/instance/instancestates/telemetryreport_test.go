package instancestates_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	telemetryapi "github.com/getsphericon/sphericon/gen/telemetry"
	"github.com/getsphericon/sphericon/internal/domain/instance/instancestates"
	"github.com/getsphericon/sphericon/internal/telemetryschema"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// receiver is a fake telemetry receiver: it keeps every body it was sent.
type receiver struct {
	*httptest.Server

	mu     sync.Mutex
	bodies [][]byte
}

func newReceiver(t *testing.T, status int) *receiver {
	t.Helper()
	r := &receiver{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		r.mu.Lock()
		r.bodies = append(r.bodies, body)
		r.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *receiver) got() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.bodies...)
}

// clock is a settable test clock.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

type rig struct {
	mod   *instancestates.Module
	recv  *receiver
	clock *clock
	db    *ent.Client
	host  instancestates.TelemetryHost
}

// newRig builds an Instance state module and a Telemetry report that would send: a production-like config (not dev) over
// the receiver, a release version and the notice acknowledged at the clock's time. edit
// changes the config, version is the running version.
// noPrep is the preparation of a case that needs none.
func noPrep(*testing.T, *rig) {}

func newRig(t *testing.T, version string, edit func(*config.Config)) *rig {
	t.Helper()
	env := testhelper.Setup(t)
	recv := newReceiver(t, http.StatusNoContent)
	cfg := &config.Config{TelemetryReceiverURL: recv.URL}
	if edit != nil {
		edit(cfg)
	}
	clk := &clock{t: time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)}
	host := testHost
	host.AppVersion = version
	client, err := telemetryapi.NewClient(cfg.TelemetryReceiverURL)
	require.NoError(t, err)
	mod := instancestates.New(env.DB, instancestates.EnvironmentOf(cfg, version, time.Time{}), clk.now,
		nil, nil, host, client, slog.Default())
	_, err = mod.AcknowledgeNotice(t.Context())
	require.NoError(t, err)
	return &rig{mod: mod, recv: recv, clock: clk, db: env.DB, host: host}
}

func (r *rig) lastSent(t *testing.T) *time.Time {
	t.Helper()
	row, err := r.mod.Ensure(t.Context())
	require.NoError(t, err)
	return row.LastReportSentAt
}

// offset is the Instance's fixed time of day for its report.
func (r *rig) offset(t *testing.T) time.Duration {
	t.Helper()
	row, err := r.mod.Ensure(t.Context())
	require.NoError(t, err)
	return instancestates.ReportOffset(row.InstanceID)
}

// firstSend moves the clock to the moment the first report is due.
func (r *rig) firstSend(t *testing.T) {
	t.Helper()
	r.clock.t = r.clock.t.Add(r.offset(t))
}

func TestSenderPostsTheReportThePreviewShowsAndRecordsWhenItWasSent(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	r.firstSend(t)
	preview, err := r.mod.TelemetryReport(t.Context())
	require.NoError(t, err)
	want, err := json.Marshal(preview)
	require.NoError(t, err)

	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))

	bodies := r.recv.got()
	require.Len(t, bodies, 1)
	require.NoError(t, telemetryschema.Validate(bodies[0]))
	assert.JSONEq(t, string(want), string(bodies[0]))
	require.NotNil(t, r.lastSent(t))
	assert.True(t, r.lastSent(t).Equal(r.clock.t))
}

func TestSenderSendsOncePerDay(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	r.firstSend(t)
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))

	r.clock.t = r.clock.t.Add(3 * time.Hour)
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Len(t, r.recv.got(), 1)

	r.clock.t = r.clock.t.Add(24 * time.Hour)
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Len(t, r.recv.got(), 2)
	assert.True(t, r.lastSent(t).Equal(r.clock.t))
}

func TestSenderFirstReportWaitsTheOffsetAfterTheNoticeAndLaterOnesKeepTheTimeOfDay(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	acked := r.clock.t
	offset := r.offset(t)

	r.clock.t = acked.Add(offset - time.Nanosecond)
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Empty(t, r.recv.got())

	r.clock.t = acked.Add(offset)
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Len(t, r.recv.got(), 1)

	nextSlot := r.clock.t.Truncate(24 * time.Hour).Add(24*time.Hour + offset)
	r.clock.t = nextSlot.Add(-time.Minute)
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Len(t, r.recv.got(), 1)

	r.clock.t = nextSlot
	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Len(t, r.recv.got(), 2)
}

func TestSenderStaysSilent(t *testing.T) {
	yes, no := true, false
	for name, c := range map[string]struct {
		version string
		edit    func(*config.Config)
		prep    func(*testing.T, *rig)
	}{
		"environment off":                   {"1.4.2", func(c *config.Config) { c.Telemetry = &no }, noPrep},
		"do not track":                      {"1.4.2", func(c *config.Config) { c.DoNotTrack = true }, noPrep},
		"do not track beats environment on": {"1.4.2", func(c *config.Config) { c.Telemetry = &yes; c.DoNotTrack = true }, noPrep},
		"switched off in the app": {"1.4.2", nil, func(t *testing.T, r *rig) {
			t.Helper()
			_, err := r.mod.SetTelemetry(t.Context(), false)
			require.NoError(t, err)
		}},
		"development environment": {"1.4.2", func(c *config.Config) { c.IsDev = true }, noPrep},
		"dev build":               {"dev", nil, noPrep},
		"prerelease build":        {"1.5.0-rc.1", nil, noPrep},
		"notice not acknowledged": {"1.4.2", nil, func(t *testing.T, r *rig) {
			t.Helper()
			require.NoError(t, r.db.InstanceState.UpdateOneID(1).ClearNoticeAcknowledgedAt().Exec(t.Context()))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, c.version, c.edit)
			r.firstSend(t)
			c.prep(t, r)

			require.NoError(t, r.mod.SendTelemetryReport(t.Context()))

			assert.Empty(t, r.recv.got())
			assert.Nil(t, r.lastSent(t))
		})
	}
}

func TestSenderSendsWhenTheEnvironmentTurnsItOnWithoutTheStaffMembersChoice(t *testing.T) {
	yes := true
	r := newRig(t, "1.4.2", func(c *config.Config) { c.Telemetry = &yes })
	r.firstSend(t)

	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))

	assert.Len(t, r.recv.got(), 1)
}

func TestSenderLeavesThingsHealthyWhenTheReceiverIsUnreachableOrRefuses(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	r.firstSend(t)
	r.recv.Close()

	require.NoError(t, r.mod.SendTelemetryReport(t.Context()))
	assert.Nil(t, r.lastSent(t))

	failing := newReceiver(t, http.StatusInternalServerError)
	client, err := telemetryapi.NewClient(failing.URL)
	require.NoError(t, err)
	sender := instancestates.New(r.db, r.mod.Environment(), r.clock.now, nil, nil, r.host, client, slog.Default())

	require.NoError(t, sender.SendTelemetryReport(t.Context()))
	assert.Nil(t, r.lastSent(t))
}
