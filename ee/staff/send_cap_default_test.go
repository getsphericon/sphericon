package staff_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/instancestate"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Instance's default Send cap (ADR 0067): a Staff member sets or clears it, and every
// Workspace created afterwards starts with it; existing Workspaces keep their own.

func defaultSendCap(t *testing.T, c *staffeeapi.Client) *staffeeapi.StaffSendCapDefault {
	t.Helper()
	res, err := c.StaffSendCapDefaultsGet(t.Context())
	require.NoError(t, err)
	d, ok := res.(*staffeeapi.StaffSendCapDefault)
	require.Truef(t, ok, "got %T", res)
	return d
}

func setDefaultSendCap(t *testing.T, c *staffeeapi.Client, sendCap int32) staffeeapi.StaffSendCapDefaultsSetRes {
	t.Helper()
	res, err := c.StaffSendCapDefaultsSet(t.Context(), &staffeeapi.StaffSendCapInput{SendCap: sendCap})
	require.NoError(t, err)
	return res
}

func clearDefaultSendCap(t *testing.T, c *staffeeapi.Client) staffeeapi.StaffSendCapDefaultsClearRes {
	t.Helper()
	res, err := c.StaffSendCapDefaultsClear(t.Context())
	require.NoError(t, err)
	return res
}

// createWorkspace creates a Workspace as John, the Verified fixture Owner, and returns its id.
func createWorkspace(t *testing.T, env *testhelper.TestEnv, name string) string {
	t.Helper()
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: name})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	return string(created.ID)
}

// sendCapOf is the Workspace's Send cap as the console shows it; nil for none.
func sendCapOf(t *testing.T, c *staffeeapi.Client, id string) *int32 {
	t.Helper()
	res, err := c.StaffWorkspacesGet(t.Context(), staffeeapi.StaffWorkspacesGetParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
	require.NoError(t, err)
	w, ok := res.(*staffeeapi.StaffWorkspaceDetailResource)
	require.Truef(t, ok, "got %T", res)
	v, ok := w.SendCap.Get()
	if !ok {
		return nil
	}
	return &v
}

func TestAStaffMemberSetsChangesAndClearsTheDefaultSendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSaaS))
	c, _ := h.staffSession(t)

	unset := defaultSendCap(t, c)
	set, ok := setDefaultSendCap(t, c, 2000).(*staffeeapi.StaffSendCapDefaultChange)
	require.True(t, ok)
	again, ok := setDefaultSendCap(t, c, 2000).(*staffeeapi.StaffSendCapDefaultChange)
	require.True(t, ok)
	cleared, ok := clearDefaultSendCap(t, c).(*staffeeapi.StaffSendCapDefaultChange)
	require.True(t, ok)

	assert.True(t, unset.SendCaps)
	assert.True(t, unset.SendCap.Null, "no default until a Staff member sets one")
	assert.True(t, set.Changed)
	assert.Equal(t, int32(2000), set.Default.SendCap.Value)
	assert.False(t, again.Changed, "idempotent")
	assert.True(t, cleared.Changed)
	assert.True(t, cleared.Default.SendCap.Null)
	assert.True(t, defaultSendCap(t, c).SendCap.Null)
}

func TestANewWorkspaceStartsWithTheDefaultAndExistingOnesKeepTheirs(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSaaS))
	c, _ := h.staffSession(t)
	h.setSendCap(t, c, fixtures.AcmePublicID, 5000)
	before := createWorkspace(t, h.env, "Before the default")

	setDefaultSendCap(t, c, 2000)
	first := createWorkspace(t, h.env, "Capped")
	setDefaultSendCap(t, c, 3000)
	second := createWorkspace(t, h.env, "Capped later")
	clearDefaultSendCap(t, c)
	third := createWorkspace(t, h.env, "Uncapped")

	assert.Nil(t, sendCapOf(t, c, before), "created while no default was set")
	assert.Equal(t, new(int32(2000)), sendCapOf(t, c, first), "the default is copied at creation")
	assert.Equal(t, new(int32(3000)), sendCapOf(t, c, second))
	assert.Nil(t, sendCapOf(t, c, third))
	assert.Equal(t, new(int32(5000)), sendCapOf(t, c, fixtures.AcmePublicID), "changing the default re-caps nobody")
}

func TestASelfHostedInstanceHasNoDefaultSendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSelfHosted))
	c, _ := h.staffSession(t)
	// test writes: a default restored from a saas dump, which no selfhosted operation can set
	h.env.DB.InstanceState.Create().SetID(1).SetDefaultSendCap(2000).
		OnConflictColumns(instancestate.FieldID).UpdateDefaultSendCap().ExecX(t.Context())

	set := setDefaultSendCap(t, c, 2000)
	cleared := clearDefaultSendCap(t, c)
	got := defaultSendCap(t, c)
	created := createWorkspace(t, h.env, "Self-hosted")

	assert.IsType(t, &staffeeapi.StaffSendCapDefaultsSetNotFound{}, set)
	assert.IsType(t, &staffeeapi.StaffSendCapDefaultsClearNotFound{}, cleared)
	assert.False(t, got.SendCaps)
	assert.True(t, got.SendCap.Null)
	assert.Nil(t, sendCapOf(t, c, created), "a Workspace created here gets no Send cap")
}
