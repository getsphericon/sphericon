package staff_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/auditentry"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func (h *harness) setSendCap(t *testing.T, c *staffeeapi.Client, id string, sendCap int32) staffeeapi.StaffWorkspacesSetSendCapRes {
	t.Helper()
	res, err := c.StaffWorkspacesSetSendCap(t.Context(), &staffeeapi.StaffSendCapInput{SendCap: sendCap},
		staffeeapi.StaffWorkspacesSetSendCapParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
	require.NoError(t, err)
	return res
}

func (h *harness) clearSendCap(t *testing.T, c *staffeeapi.Client, id string) staffeeapi.StaffWorkspacesClearSendCapRes {
	t.Helper()
	res, err := c.StaffWorkspacesClearSendCap(t.Context(), staffeeapi.StaffWorkspacesClearSendCapParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
	require.NoError(t, err)
	return res
}

// A Staff member sets, changes and removes a Workspace's Send cap (ADR 0067); each change is in
// the Workspace's Audit log with the Staff member as actor and the old and new values, and a
// change to the same value changes nothing.
func TestAStaffMemberSetsChangesAndRemovesASendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSaaS))
	c, _ := h.staffSession(t)

	set, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 1000).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	again, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 1000).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	raised, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 5000).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	cleared, ok := h.clearSendCap(t, c, fixtures.AcmePublicID).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	h.env.DeliverToEE(t)

	assert.True(t, set.Changed)
	assert.EqualValues(t, 1000, set.Workspace.SendCap.Value)
	assert.False(t, again.Changed, "idempotent")
	assert.EqualValues(t, 5000, raised.Workspace.SendCap.Value)
	assert.True(t, cleared.Changed)
	assert.False(t, cleared.Workspace.SendCap.IsSet(), "no Send cap: only the Provider limits slow it")
	entries := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.Action(events.ActionWorkspaceSendCap)).Order(auditentry.ByID()).AllX(t.Context())
	require.Len(t, entries, 3)
	assert.Equal(t, string(events.ActorStaff), entries[1].ActorKind)
	diff, err := json.Marshal(entries[1].Diff)
	require.NoError(t, err)
	assert.JSONEq(t, `{"send_cap":{"from":1000,"to":5000}}`, string(diff))
}

// An Instance without Send caps has no such operation, whatever the License key, and the console
// shows no Send cap even if one is stored.
func TestASelfHostedInstanceHasNoSendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSelfHosted))
	c, _ := h.staffSession(t)

	set := h.setSendCap(t, c, fixtures.AcmePublicID, 1000)
	cleared := h.clearSendCap(t, c, fixtures.AcmePublicID)
	stored := 10
	// test writes: a Send cap restored from a saas dump, which no selfhosted operation can set
	h.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(stored).ExecX(t.Context())
	got, err := c.StaffWorkspacesGet(t.Context(), staffeeapi.StaffWorkspacesGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	assert.IsType(t, &staffeeapi.StaffWorkspacesSetSendCapNotFound{}, set)
	assert.IsType(t, &staffeeapi.StaffWorkspacesClearSendCapNotFound{}, cleared)
	detail, ok := got.(*staffeeapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	assert.False(t, detail.SendCap.IsSet())
	assert.False(t, detail.SentLast24h.IsSet(), "nothing to count against")
}

// The console shows the Workspace's sends in the last 24 hours next to its Send cap, to judge
// whether it is near the cap.
func TestTheConsoleShowsTheLastDaySendsNextToTheSendCap(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSaaS))
	c, _ := h.staffSession(t)
	h.setSendCap(t, c, fixtures.AcmePublicID, 1000)

	got, err := c.StaffWorkspacesGet(t.Context(), staffeeapi.StaffWorkspacesGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	detail, ok := got.(*staffeeapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	assert.True(t, detail.SendCaps)
	assert.EqualValues(t, 1000, detail.SendCap.Value)
	assert.EqualValues(t, 3, detail.SentLast24h.Value, "every Integration's sends of the last 24 hours (fixtures)")
}
