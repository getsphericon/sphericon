package staff_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/workspace"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Workspace created on an Instance that has Send caps starts with the Instance's default Send cap
// (ADR 0067), set by a Staff member in the core console; changing the default leaves existing
// Workspaces as they are, and a self-hosted Instance gives a new Workspace none.

// consoles logs the enrolled fixture Staff member in once and returns a client of the core console
// (the Instance's settings) and one of its Enterprise sections (the Workspaces), on one session.
func (h *harness) consoles(t *testing.T) (core *staffapi.Client, enterprise *staffeeapi.Client) {
	t.Helper()
	_, cookie := h.staffSession(t)
	return h.env.StaffWithToken(t, cookie.Value), h.env.StaffEnterpriseWithToken(t, cookie.Value)
}

func (h *harness) setDefaultSendCap(t *testing.T, c *staffapi.Client, perDay int32) {
	t.Helper()
	res, err := c.StaffInstanceSetDefaultSendCap(t.Context(), &staffapi.StaffDefaultSendCapInput{PerDay: staffapi.MaxPerDay(perDay)})
	require.NoError(t, err)
	require.IsType(t, &staffapi.StaffDefaultSendCapChange{}, res)
}

func (h *harness) clearDefaultSendCap(t *testing.T, c *staffapi.Client) {
	t.Helper()
	res, err := c.StaffInstanceClearDefaultSendCap(t.Context())
	require.NoError(t, err)
	require.IsType(t, &staffapi.StaffDefaultSendCapChange{}, res)
}

// createWorkspace creates a Workspace as John, its Owner, and returns its id.
func (h *harness) createWorkspace(t *testing.T, name string) string {
	t.Helper()
	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: name})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	return string(created.ID)
}

// sendCapOf is the Workspace's Send cap as the console shows it.
func (h *harness) sendCapOf(t *testing.T, c *staffeeapi.Client, id string) staffeeapi.NilMaxPerDay {
	t.Helper()
	w := h.workspaceDetail(t, c, id)
	require.True(t, w.SendCap.IsSet(), "an Instance with Send caps shows the Send cap")
	return w.SendCap.Value.PerDay
}

func TestANewWorkspaceStartsWithTheDefaultSendCapAndKeepsItWhenTheDefaultChanges(t *testing.T) {
	h := newHarness(t)
	core, ee := h.consoles(t)

	none := h.createWorkspace(t, "Before any default")
	h.setDefaultSendCap(t, core, 500)
	capped := h.createWorkspace(t, "Under the default")

	noneCap := h.sendCapOf(t, ee, none)
	cappedCap := h.sendCapOf(t, ee, capped)
	acmeCap := h.sendCapOf(t, ee, fixtures.AcmePublicID)
	assert.True(t, noneCap.IsNull(), "a Workspace created with no default has no Send cap")
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(500), cappedCap, "the default is copied at creation")
	assert.True(t, acmeCap.IsNull(), "setting the default re-caps no existing Workspace")

	h.setDefaultSendCap(t, core, 2000)
	raised := h.createWorkspace(t, "Under the raised default")
	h.clearDefaultSendCap(t, core)
	uncapped := h.createWorkspace(t, "After the default is cleared")

	cappedLater := h.sendCapOf(t, ee, capped)
	raisedCap := h.sendCapOf(t, ee, raised)
	uncappedCap := h.sendCapOf(t, ee, uncapped)
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(500), cappedLater, "changing the default leaves the Workspace's own")
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(2000), raisedCap, "clearing the default leaves the Workspace's own")
	assert.True(t, uncappedCap.IsNull(), "a Workspace created after the default is cleared has none")
}

func TestASignedUpUsersFirstWorkspaceStartsWithTheDefaultSendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithConfig(func(c *config.Config) { c.RequireEmailVerification = false }))
	core, ee := h.consoles(t)
	h.setDefaultSendCap(t, core, 750)

	res, err := h.env.SiteAnonymous(t).SiteAuthSignUp(t.Context(), &siteapi.SiteSignUpInput{
		Name: "Grace Hopper", Email: "grace@example.com", Password: siteapi.NewOptString("grace-password-1"),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthSignUpAccepted{}, res)
	spaces, err := h.env.SiteActor(t, "grace@example.com").SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)

	first := h.sendCapOf(t, ee, string(spaces[0].ID))
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(750), first)
}

// A self-hosted Instance has no Send caps: a default stored in its database (say, from a restored
// dump) is not copied onto a new Workspace.
func TestASelfHostedInstanceGivesANewWorkspaceNoSendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSelfHosted))
	_, err := h.env.Instance.Ensure(t.Context())
	require.NoError(t, err)
	h.env.DB.InstanceState.UpdateOneID(1).SetDefaultSendCap(500).ExecX(t.Context())

	id := h.createWorkspace(t, "Self-hosted")

	w, err := h.env.DB.Workspace.Query().Where(workspace.PublicID(id)).Only(t.Context())
	require.NoError(t, err)
	assert.Nil(t, w.SendCap)
}
