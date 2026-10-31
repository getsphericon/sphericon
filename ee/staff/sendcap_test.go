package staff_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The Send cap (ADR 0067) a Staff member sets on a Workspace from the console, on an Instance that
// has Send caps (the harness's default Mode), and its absence on a self-hosted one.

func (h *harness) setSendCap(t *testing.T, c *staffeeapi.Client, id string, perDay int32) staffeeapi.StaffWorkspacesSetSendCapRes {
	t.Helper()
	res, err := c.StaffWorkspacesSetSendCap(t.Context(), &staffeeapi.StaffSendCapInput{PerDay: staffeeapi.MaxPerDay(perDay)},
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

func (h *harness) workspaceDetail(t *testing.T, c *staffeeapi.Client, id string) *staffeeapi.StaffWorkspaceDetailResource {
	t.Helper()
	res, err := c.StaffWorkspacesGet(t.Context(), staffeeapi.StaffWorkspacesGetParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
	require.NoError(t, err)
	w, ok := res.(*staffeeapi.StaffWorkspaceDetailResource)
	require.True(t, ok)
	return w
}

// sendCapChanges are the Send cap Audit entries published so far, as JSON of their diffs.
func sendCapChanges(t *testing.T, env *testhelper.TestEnv) (entries []*events.AuditEntry, diffs []string) {
	t.Helper()
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		e, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		if e.Action != events.ActionWorkspaceSendCapChange {
			continue
		}
		b, err := json.Marshal(e.Diff)
		require.NoError(t, err)
		entries, diffs = append(entries, e), append(diffs, string(b))
	}
	return entries, diffs
}

func TestAStaffMemberSetsChangesAndClearsAWorkspacesSendCap(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	before := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	require.True(t, before.SendCap.IsSet(), "an Instance with Send caps shows the Send cap")
	assert.True(t, before.SendCap.Value.PerDay.IsNull(), "a Workspace starts with none")

	set, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 500).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	assert.True(t, set.Changed)
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(500), set.SendCap.PerDay)

	changed, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 2000).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	afterChange := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	assert.True(t, changed.Changed)
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(2000), afterChange.SendCap.Value.PerDay)

	cleared, ok := h.clearSendCap(t, c, fixtures.AcmePublicID).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	afterClear := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	assert.True(t, cleared.Changed)
	assert.True(t, cleared.SendCap.PerDay.IsNull())
	assert.True(t, afterClear.SendCap.Value.PerDay.IsNull())

	entries, diffs := sendCapChanges(t, h.env)
	assert.Equal(t, []string{
		`{"send_cap":{"to":500}}`,
		`{"send_cap":{"from":500,"to":2000}}`,
		`{"send_cap":{"from":2000}}`,
	}, diffs, "each change records the old and the new value, absent where there is none")
	for _, e := range entries {
		assert.Equal(t, int64(fixtures.AcmeID), e.WorkspaceID, "in the Workspace's own Audit log")
		assert.Equal(t, events.Actor{Kind: events.ActorStaff, ID: strconv.Itoa(fixtures.StaffEnrolledID)}, e.Actor,
			"the signed-in Staff member is the actor")
	}
}

func TestSettingTheSameSendCapAndClearingNoneChangeNothing(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	none, ok := h.clearSendCap(t, c, fixtures.AcmePublicID).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	assert.False(t, none.Changed, "a Workspace with no Send cap stays as it was")

	h.setSendCap(t, c, fixtures.AcmePublicID, 500)
	same, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 500).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	assert.False(t, same.Changed)
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(500), same.SendCap.PerDay)

	_, diffs := sendCapChanges(t, h.env)
	assert.Equal(t, []string{`{"send_cap":{"to":500}}`}, diffs, "only the real change is recorded")
}

// The Staff member sees the Workspace's sends over the last 24 hours next to its Send cap, to
// judge whether it is near the cap: on the Workspace page and in a change's answer, with a Send
// cap or without one.
func TestTheSendCapShowsTheWorkspacesSendsOfTheLast24Hours(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	// Three sends inside the window: two through the default Integration and one through the SES
	// one; the older ones are outside it.
	const sent = 3
	uncapped := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	assert.Equal(t, int32(sent), uncapped.SendCap.Value.SentLast24h, "without a Send cap too")

	set, ok := h.setSendCap(t, c, fixtures.AcmePublicID, 500).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	capped := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	assert.Equal(t, staffeeapi.StaffSendCap{PerDay: staffeeapi.NewNilMaxPerDay(500), SentLast24h: sent}, set.SendCap)
	assert.Equal(t, set.SendCap, capped.SendCap.Value)

	cleared, ok := h.clearSendCap(t, c, fixtures.AcmePublicID).(*staffeeapi.StaffSendCapChange)
	require.True(t, ok)
	assert.Equal(t, int32(sent), cleared.SendCap.SentLast24h)

	globex := h.workspaceDetail(t, c, fixtures.GlobexPublicID).SendCap.Value
	assert.Equal(t, int32(1), globex.SentLast24h, "each Workspace counts only its own sends")
}

func TestTheSendCapOfAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	set := h.setSendCap(t, c, "Zz9Zz9Zz", 500)
	cleared := h.clearSendCap(t, c, "Zz9Zz9Zz")
	assert.IsType(t, &staffeeapi.StaffWorkspacesSetSendCapNotFound{}, set)
	assert.IsType(t, &staffeeapi.StaffWorkspacesClearSendCapNotFound{}, cleared)
}

// A self-hosted Instance has no Send cap: its endpoints are not found whatever the License key
// (the harness's key has every feature), and the Workspace page has no Send cap to show.
func TestASelfHostedInstanceHasNoSendCapInTheConsole(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSelfHosted))
	c, _ := h.staffSession(t)
	h.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(500).ExecX(t.Context())

	set := h.setSendCap(t, c, fixtures.AcmePublicID, 1000)
	cleared := h.clearSendCap(t, c, fixtures.AcmePublicID)
	detail := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	entries, _ := sendCapChanges(t, h.env)
	assert.IsType(t, &staffeeapi.StaffWorkspacesSetSendCapNotFound{}, set)
	assert.IsType(t, &staffeeapi.StaffWorkspacesClearSendCapNotFound{}, cleared)
	assert.False(t, detail.SendCap.IsSet(), "even a stored value is not shown")
	assert.Empty(t, entries)
}
