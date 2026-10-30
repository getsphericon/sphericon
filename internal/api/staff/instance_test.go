package staff_test

import (
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/licensekey"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/telemetryschema"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Instance's own settings are a core section of the console (ADR 0063): a signed-in Staff
// member reads and switches the Telemetry report, acknowledges its notice and previews it
// (ADR 0032). Every test here runs on a self-hosted Instance without a License key unless it
// says otherwise.

func instanceState(t *testing.T, c *staffapi.Client) *staffapi.StaffInstanceState {
	t.Helper()
	res, err := c.StaffInstanceGetState(t.Context())
	require.NoError(t, err)
	state, ok := res.(*staffapi.StaffInstanceState)
	require.Truef(t, ok, "expected the state, got %T", res)
	return state
}

func setTelemetry(t *testing.T, c *staffapi.Client, enabled bool) staffapi.StaffInstanceSetTelemetryRes {
	t.Helper()
	res, err := c.StaffInstanceSetTelemetry(t.Context(), &staffapi.StaffInstanceTelemetryInput{Enabled: enabled})
	require.NoError(t, err)
	return res
}

func TestTheTelemetryReportIsOnAndItsNoticeUnacknowledgedAtFirst(t *testing.T) {
	h := newHarness(t)

	state := instanceState(t, h.console(t))

	assert.True(t, state.Telemetry.Enabled)
	assert.False(t, state.Telemetry.Locked)
	assert.False(t, state.Telemetry.LockedBy.Set)
	assert.False(t, state.NoticeAcknowledged)
	assert.NotEmpty(t, state.TelemetrySchemaUrl)
	row, err := h.env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.NotEmpty(t, row.InstanceID)
	assert.True(t, row.TelemetryEnabled)
}

func TestAStaffMemberSwitchesTheTelemetryReportOffAndOn(t *testing.T) {
	h := newHarness(t)
	c := h.console(t)

	off, ok := setTelemetry(t, c, false).(*staffapi.StaffInstanceState)
	require.True(t, ok)
	assert.False(t, off.Telemetry.Enabled)
	row, err := h.env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.False(t, row.TelemetryEnabled)
	assert.False(t, instanceState(t, c).Telemetry.Enabled)

	setTelemetry(t, c, true)
	assert.True(t, instanceState(t, c).Telemetry.Enabled)
}

func TestAcknowledgingTheNoticeKeepsTheFirstTime(t *testing.T) {
	h := newHarness(t)
	first := h.clock.now()

	res, err := h.console(t).StaffInstanceAcknowledgeNotice(t.Context())
	require.NoError(t, err)
	state, ok := res.(*staffapi.StaffInstanceState)
	require.True(t, ok)
	assert.True(t, state.NoticeAcknowledged)

	// Two days on, past the first session: a Staff member signs in again and acknowledges again.
	h.clock.t = h.clock.t.Add(48 * time.Hour)
	_, err = h.console(t).StaffInstanceAcknowledgeNotice(t.Context())
	require.NoError(t, err)
	row, err := h.env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, row.NoticeAcknowledgedAt)
	assert.WithinDuration(t, first, *row.NoticeAcknowledgedAt, time.Millisecond)
}

func TestTheTelemetrySettingFollowsTheEnvironment(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name       string
		telemetry  *bool
		doNotTrack bool
		stored     []bool // none: no stored setting; one: its value
		enabled    bool
		locked     bool
		lockedBy   staffapi.StaffInstanceTelemetryLock
	}{
		{name: "unset is on", enabled: true},
		{name: "stored off", stored: []bool{false}, enabled: false},
		{name: "stored on", stored: []bool{true}, enabled: true},
		{name: "true is on and locked over a stored off", telemetry: &yes, stored: []bool{false}, enabled: true, locked: true, lockedBy: staffapi.StaffInstanceTelemetryLockEnvironment},
		{name: "false is off and locked over a stored on", telemetry: &no, stored: []bool{true}, locked: true, lockedBy: staffapi.StaffInstanceTelemetryLockEnvironment},
		{name: "do not track is off and locked", doNotTrack: true, locked: true, lockedBy: staffapi.StaffInstanceTelemetryLockDoNotTrack},
		{name: "do not track never switches on, even over true", telemetry: &yes, doNotTrack: true, stored: []bool{true}, locked: true, lockedBy: staffapi.StaffInstanceTelemetryLockDoNotTrack},
		{name: "false wins over do not track", telemetry: &no, doNotTrack: true, locked: true, lockedBy: staffapi.StaffInstanceTelemetryLockEnvironment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, testhelper.WithConfig(func(c *config.Config) {
				c.Telemetry = tc.telemetry
				c.DoNotTrack = tc.doNotTrack
			}))
			for _, stored := range tc.stored {
				row, err := h.env.Instance.Ensure(t.Context())
				require.NoError(t, err)
				require.NoError(t, h.env.DB.InstanceState.UpdateOneID(row.ID).SetTelemetryEnabled(stored).Exec(t.Context()))
			}

			state := instanceState(t, h.console(t))

			assert.Equal(t, tc.enabled, state.Telemetry.Enabled)
			assert.Equal(t, tc.locked, state.Telemetry.Locked)
			assert.Equal(t, tc.locked, state.Telemetry.LockedBy.Set)
			assert.Equal(t, tc.lockedBy, state.Telemetry.LockedBy.Value)
		})
	}
}

// The saas Mode keeps the report off: the switch shows as locked and a change answers 409.
func TestALockedSettingIsRefusedAndShownLocked(t *testing.T) {
	h := newLicensedHarness(t, testhelper.WithMode(config.ModeSaaS))
	c := h.console(t)

	conflict, ok := setTelemetry(t, c, true).(*staffapi.StaffInstanceSetTelemetryConflict)

	require.True(t, ok, "expected a conflict")
	assert.NotEmpty(t, conflict.Detail.Value)
	state := instanceState(t, c)
	assert.False(t, state.Telemetry.Enabled)
	assert.True(t, state.Telemetry.Locked)
	assert.Equal(t, staffapi.StaffInstanceTelemetryLockEnvironment, state.Telemetry.LockedBy.Value)
	row, err := h.env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.True(t, row.TelemetryEnabled, "the stored choice is untouched")
}

func TestThePreviewIsTheValidReportAndWorksWithSendingOff(t *testing.T) {
	h := newHarness(t, testhelper.WithConfig(func(c *config.Config) { c.DoNotTrack = true }))
	c := h.console(t)
	require.False(t, instanceState(t, c).Telemetry.Enabled)

	res, err := c.StaffInstancePreviewTelemetryReport(t.Context())

	require.NoError(t, err)
	preview, ok := res.(*staffapi.StaffInstancePreviewTelemetryReportOK)
	require.Truef(t, ok, "expected the report, got %T", res)
	body, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NoError(t, telemetryschema.Validate(body))
	row, err := h.env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.JSONEq(t, `"`+row.InstanceID+`"`, string((*preview)["instanceId"]))
}

// releases is a GitHub Releases source whose latest release is tag.
func releases(t *testing.T, tag string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "assets": []any{}}))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestTheConsoleShowsANewerRelease(t *testing.T) {
	url := releases(t, "v0.2.0")
	h := newHarness(t, testhelper.WithRunningVersion("0.1.0"), testhelper.WithConfig(func(c *config.Config) {
		c.GitHubAPIURL = url
		c.ReleaseCheck = true
		c.IsDev = false // the Release check never runs in a development or test environment
	}))
	require.NoError(t, h.env.Instance.CheckRelease(t.Context()))

	state := instanceState(t, h.console(t))

	assert.Equal(t, "0.2.0", state.LatestRelease.Value)
}

// licensed is a harness on a self-hosted Instance whose License key expires at expires.
func licensed(t *testing.T, expires time.Time) *harness {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	key, err := licensekey.Issue(priv, &expires, licensekey.FeatureStaff)
	require.NoError(t, err)
	return newLicensedHarness(t, testhelper.WithMode(config.ModeSelfHosted), testhelper.WithLicenseKey(pub, key))
}

// License expiry is shown in the console only (ADR 0035): no Workspace sees it.
func TestTheConsoleShowsWhenTheLicenseKeyExpired(t *testing.T) {
	expiredOn := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	h := licensed(t, expiredOn)

	state := instanceState(t, h.console(t))

	require.True(t, state.LicenseExpiredAt.Set)
	assert.True(t, time.Time(state.LicenseExpiredAt.Value).Equal(expiredOn))
}

func TestAValidLicenseKeyShowsNoLapse(t *testing.T) {
	h := licensed(t, time.Now().Add(24*time.Hour))

	assert.False(t, instanceState(t, h.console(t)).LicenseExpiredAt.Set)
}

func TestTheInstanceSectionNeedsASignedInStaffMember(t *testing.T) {
	h := newHarness(t)
	c := h.env.StaffAnonymous(t)

	state, err := c.StaffInstanceGetState(t.Context())
	require.NoError(t, err)
	requireProblem(t, state, 401)
	ack, err := c.StaffInstanceAcknowledgeNotice(t.Context())
	require.NoError(t, err)
	requireProblem(t, ack, 401)
	row, err := h.env.Instance.Ensure(t.Context())
	require.NoError(t, err)
	assert.Nil(t, row.NoticeAcknowledgedAt)
}
