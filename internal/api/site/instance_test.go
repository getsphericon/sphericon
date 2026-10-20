package site_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/telemetryschema"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func instanceState(t *testing.T, c *siteapi.Client) *siteapi.SiteInstanceState {
	t.Helper()
	res, err := c.SiteInstanceGetState(t.Context())
	require.NoError(t, err)
	state, ok := res.(*siteapi.SiteInstanceState)
	require.True(t, ok, "expected the state, got %T", res)
	return state
}

func requireInstanceForbidden(t *testing.T, c *siteapi.Client) {
	t.Helper()
	res, err := c.SiteInstanceGetState(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
	set, err := c.SiteInstanceSetTelemetry(t.Context(), &siteapi.SiteInstanceTelemetryInput{Enabled: false})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInstanceSetTelemetryForbidden{}, set)
	ack, err := c.SiteInstanceAcknowledgeNotice(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, ack)
	preview, err := c.SiteInstancePreviewTelemetryReport(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, preview)
}

func TestSiteInstanceStateDefaultsToOnAndUnacknowledged(t *testing.T) {
	env := testhelper.Setup(t)

	state := instanceState(t, env.SiteActor(t, fixtures.OwnerJohnEmail))

	assert.True(t, state.Telemetry.Enabled)
	assert.False(t, state.Telemetry.Locked)
	assert.False(t, state.Telemetry.LockedBy.Set)
	assert.False(t, state.NoticeAcknowledged)
	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.NotEmpty(t, row.InstanceID)
	assert.Nil(t, row.TelemetryEnabled)
}

func TestSiteInstanceKeepsOneInstanceIDAcrossReads(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	instanceState(t, john)
	first, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	instanceState(t, john)
	second, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)

	assert.Equal(t, first.InstanceID, second.InstanceID)
}

func TestSiteInstanceOwnerSwitchesTelemetryOffAndOn(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := john.SiteInstanceSetTelemetry(t.Context(), &siteapi.SiteInstanceTelemetryInput{Enabled: false})
	require.NoError(t, err)
	off, ok := res.(*siteapi.SiteInstanceState)
	require.True(t, ok)
	assert.False(t, off.Telemetry.Enabled)
	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, row.TelemetryEnabled)
	assert.False(t, *row.TelemetryEnabled)
	assert.False(t, instanceState(t, john).Telemetry.Enabled)

	_, err = john.SiteInstanceSetTelemetry(t.Context(), &siteapi.SiteInstanceTelemetryInput{Enabled: true})
	require.NoError(t, err)
	assert.True(t, instanceState(t, john).Telemetry.Enabled)
}

func TestSiteInstanceAcknowledgeNoticeKeepsTheFirstTime(t *testing.T) {
	first := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	now := first
	env := testhelper.Setup(t, testhelper.WithClock(func() time.Time { return now }))
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := john.SiteInstanceAcknowledgeNotice(t.Context())
	require.NoError(t, err)
	state, ok := res.(*siteapi.SiteInstanceState)
	require.True(t, ok)
	assert.True(t, state.NoticeAcknowledged)

	now = first.Add(48 * time.Hour)
	_, err = john.SiteInstanceAcknowledgeNotice(t.Context())
	require.NoError(t, err)
	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, row.NoticeAcknowledgedAt)
	assert.True(t, row.NoticeAcknowledgedAt.Equal(first))
	assert.True(t, instanceState(t, john).NoticeAcknowledged)
}

func TestSiteInstanceRefusesEveryoneButOwnersOfTheOldestWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	t.Run("a member of the oldest workspace", func(t *testing.T) {
		requireInstanceForbidden(t, env.SiteActor(t, fixtures.MemberMaryEmail))
	})
	t.Run("an owner of another workspace", func(t *testing.T) {
		requireInstanceForbidden(t, env.SiteActor(t, fixtures.OwnerJaneEmail))
	})
	t.Run("a user with no membership", func(t *testing.T) {
		requireInstanceForbidden(t, env.SiteActor(t, fixtures.OutsiderOscarEmail))
	})
	t.Run("an unauthenticated caller", func(t *testing.T) {
		_, err := env.SiteAnonymous(t).SiteInstanceGetState(t.Context())
		require.Error(t, err)
	})
}

func TestSiteInstanceEveryOwnerOfTheOldestWorkspaceMayDecideAndATransferFollowsTheRole(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := john.SiteMembershipsUpdate(t.Context(), &siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleOwner},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
	require.NoError(t, err)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	res, err := mary.SiteInstanceSetTelemetry(t.Context(), &siteapi.SiteInstanceTelemetryInput{Enabled: false})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteInstanceState{}, res)
	assert.False(t, instanceState(t, john).Telemetry.Enabled)

	// Mary takes over: the other Owner demotes John and he loses the Instance.
	_, err = mary.SiteMembershipsUpdate(t.Context(), &siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeOwnerMembershipID)})
	require.NoError(t, err)
	requireInstanceForbidden(t, john)
	instanceState(t, mary)
}

func TestSiteInstanceTelemetrySettingFollowsTheEnvironment(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name       string
		telemetry  *bool
		doNotTrack bool
		stored     []bool // none: no stored setting; one: its value
		enabled    bool
		locked     bool
		lockedBy   siteapi.SiteInstanceTelemetryLock
	}{
		{name: "unset is on", enabled: true},
		{name: "stored off", stored: []bool{false}, enabled: false},
		{name: "stored on", stored: []bool{true}, enabled: true},
		{name: "true is on and locked over a stored off", telemetry: &yes, stored: []bool{false}, enabled: true, locked: true, lockedBy: siteapi.SiteInstanceTelemetryLockEnvironment},
		{name: "false is off and locked over a stored on", telemetry: &no, stored: []bool{true}, locked: true, lockedBy: siteapi.SiteInstanceTelemetryLockEnvironment},
		{name: "do not track is off and locked", doNotTrack: true, locked: true, lockedBy: siteapi.SiteInstanceTelemetryLockDoNotTrack},
		{name: "do not track never switches on, even over true", telemetry: &yes, doNotTrack: true, stored: []bool{true}, locked: true, lockedBy: siteapi.SiteInstanceTelemetryLockDoNotTrack},
		{name: "false wins over do not track", telemetry: &no, doNotTrack: true, locked: true, lockedBy: siteapi.SiteInstanceTelemetryLockEnvironment},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) {
				c.Telemetry = tc.telemetry
				c.DoNotTrack = tc.doNotTrack
			}))
			john := env.SiteActor(t, fixtures.OwnerJohnEmail)
			for _, stored := range tc.stored {
				require.NoError(t, env.DB.InstanceState.Create().SetID(1).SetTelemetryEnabled(stored).Exec(t.Context()))
			}

			state := instanceState(t, john)

			assert.Equal(t, tc.enabled, state.Telemetry.Enabled)
			assert.Equal(t, tc.locked, state.Telemetry.Locked)
			assert.Equal(t, tc.locked, state.Telemetry.LockedBy.Set)
			assert.Equal(t, tc.lockedBy, state.Telemetry.LockedBy.Value)
		})
	}
}

func TestSiteInstanceRefusesToChangeALockedSetting(t *testing.T) {
	yes := true
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.Telemetry = &yes }))
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := john.SiteInstanceSetTelemetry(t.Context(), &siteapi.SiteInstanceTelemetryInput{Enabled: false})

	require.NoError(t, err)
	conflict, ok := res.(*siteapi.SiteInstanceSetTelemetryConflict)
	require.True(t, ok, "expected a conflict, got %T", res)
	assert.NotEmpty(t, conflict.Detail.Value)
	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.Nil(t, row.TelemetryEnabled)
	assert.True(t, instanceState(t, john).Telemetry.Enabled)
}

func TestSiteInstancePreviewIsTheValidReportAndWorksWithSendingOff(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.DoNotTrack = true }))
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.False(t, instanceState(t, john).Telemetry.Enabled)

	res, err := john.SiteInstancePreviewTelemetryReport(t.Context())

	require.NoError(t, err)
	preview, ok := res.(*siteapi.SiteInstancePreviewTelemetryReportOK)
	require.True(t, ok, "expected the report, got %T", res)
	body, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NoError(t, telemetryschema.Validate(body))
	row, err := env.DB.InstanceState.Get(t.Context(), 1)
	require.NoError(t, err)
	assert.JSONEq(t, `"`+row.InstanceID+`"`, string((*preview)["instanceId"]))
	assert.JSONEq(t, `"1-9"`, string((*preview)["workspaces"]))
}
