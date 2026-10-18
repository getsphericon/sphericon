package instance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/instance"
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
		body, _ := io.ReadAll(req.Body)
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
	mod    *instance.Module
	sender *instance.Sender
	recv   *receiver
	clock  *clock
	db     *ent.Client
}

func noJitter() time.Duration { return 0 }

// newRig builds a Module and Sender that would send: a production-like config (not dev) over
// the receiver, a release version, the notice acknowledged and no jitter. edit changes the
// config, version is the running version.
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
	mod := instance.New(env.DB, instance.EnvironmentOf(cfg), host, clk.now)
	sender, err := instance.NewSender(mod, cfg, instance.WithJitter(noJitter))
	require.NoError(t, err)
	_, err = mod.AcknowledgeNotice(t.Context())
	require.NoError(t, err)
	return &rig{mod: mod, sender: sender, recv: recv, clock: clk, db: env.DB}
}

func (r *rig) lastSent(t *testing.T) *time.Time {
	t.Helper()
	row, err := r.mod.Ensure(t.Context())
	require.NoError(t, err)
	return row.LastReportSentAt
}

func TestSenderPostsTheReportThePreviewShowsAndRecordsWhenItWasSent(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	preview, err := r.mod.Report(t.Context())
	require.NoError(t, err)
	want, err := json.Marshal(preview)
	require.NoError(t, err)

	require.NoError(t, r.sender.Run(t.Context()))

	bodies := r.recv.got()
	require.Len(t, bodies, 1)
	require.NoError(t, telemetryschema.Validate(bodies[0]))
	assert.JSONEq(t, string(want), string(bodies[0]))
	require.NotNil(t, r.lastSent(t))
	assert.True(t, r.lastSent(t).Equal(r.clock.t))
}

func TestSenderSendsOncePerDay(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	require.NoError(t, r.sender.Run(t.Context()))

	r.clock.t = r.clock.t.Add(3 * time.Hour)
	require.NoError(t, r.sender.Run(t.Context()))
	assert.Len(t, r.recv.got(), 1)

	r.clock.t = r.clock.t.Add(24 * time.Hour)
	require.NoError(t, r.sender.Run(t.Context()))
	assert.Len(t, r.recv.got(), 2)
	assert.True(t, r.lastSent(t).Equal(r.clock.t))
}

func TestSenderJitterBringsTheNextSendForwardWithinTheDay(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	jittered, err := instance.NewSender(r.mod, &config.Config{TelemetryReceiverURL: r.recv.URL},
		instance.WithJitter(func() time.Duration { return 2 * time.Hour }))
	require.NoError(t, err)
	require.NoError(t, r.sender.Run(t.Context()))

	r.clock.t = r.clock.t.Add(21 * time.Hour)
	require.NoError(t, jittered.Run(t.Context()))
	assert.Len(t, r.recv.got(), 1)

	r.clock.t = r.clock.t.Add(time.Hour)
	require.NoError(t, jittered.Run(t.Context()))
	assert.Len(t, r.recv.got(), 2)
}

func TestSenderStaysSilent(t *testing.T) {
	yes, no := true, false
	for name, c := range map[string]struct {
		version string
		edit    func(*config.Config)
		prep    func(*testing.T, *rig)
	}{
		"environment off":                   {"1.4.2", func(c *config.Config) { c.Telemetry = &no }, nil},
		"do not track":                      {"1.4.2", func(c *config.Config) { c.DoNotTrack = true }, nil},
		"do not track beats environment on": {"1.4.2", func(c *config.Config) { c.Telemetry = &yes; c.DoNotTrack = true }, nil},
		"switched off in the app": {"1.4.2", nil, func(t *testing.T, r *rig) {
			_, err := r.mod.SetTelemetry(t.Context(), false)
			require.NoError(t, err)
		}},
		"development environment": {"1.4.2", func(c *config.Config) { c.IsDev = true }, nil},
		"dev build":               {"dev", nil, nil},
		"prerelease build":        {"1.5.0-rc.1", nil, nil},
		"notice not acknowledged": {"1.4.2", nil, func(t *testing.T, r *rig) {
			require.NoError(t, r.db.InstanceState.UpdateOneID(1).ClearNoticeAcknowledgedAt().Exec(t.Context()))
		}},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, c.version, c.edit)
			if c.prep != nil {
				c.prep(t, r)
			}

			require.NoError(t, r.sender.Run(t.Context()))

			assert.Empty(t, r.recv.got())
			assert.Nil(t, r.lastSent(t))
		})
	}
}

func TestSenderSendsWhenTheEnvironmentTurnsItOnWithoutTheOwnersChoice(t *testing.T) {
	yes := true
	r := newRig(t, "1.4.2", func(c *config.Config) { c.Telemetry = &yes })

	require.NoError(t, r.sender.Run(t.Context()))

	assert.Len(t, r.recv.got(), 1)
}

func TestSenderLeavesThingsHealthyWhenTheReceiverIsUnreachableOrRefuses(t *testing.T) {
	r := newRig(t, "1.4.2", nil)
	r.recv.Close()

	require.NoError(t, r.sender.Run(t.Context()))
	assert.Nil(t, r.lastSent(t))

	failing := newReceiver(t, http.StatusInternalServerError)
	sender, err := instance.NewSender(r.mod, &config.Config{TelemetryReceiverURL: failing.URL})
	require.NoError(t, err)

	require.NoError(t, sender.Run(t.Context()))
	assert.Nil(t, r.lastSent(t))
}
