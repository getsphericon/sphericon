package staff_test

import (
	"encoding/json"
	"net/http"
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

func (h *harness) trySetSendCap(t *testing.T, c *staffeeapi.Client, id string, perDay int32) (*staffeeapi.StaffSendCapChange, error) {
	t.Helper()
	return c.StaffWorkspacesSetSendCap(t.Context(), &staffeeapi.StaffSendCapInput{PerDay: staffeeapi.MaxPerDay(perDay)},
		staffeeapi.StaffWorkspacesSetSendCapParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
}

func (h *harness) tryClearSendCap(t *testing.T, c *staffeeapi.Client, id string) (*staffeeapi.StaffSendCapChange, error) {
	t.Helper()
	return c.StaffWorkspacesClearSendCap(t.Context(), staffeeapi.StaffWorkspacesClearSendCapParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
}

// setSendCap is trySetSendCap for a call that is expected to succeed.
func (h *harness) setSendCap(t *testing.T, c *staffeeapi.Client, id string, perDay int32) *staffeeapi.StaffSendCapChange {
	t.Helper()
	res, err := h.trySetSendCap(t, c, id, perDay)
	require.NoError(t, err)
	return res
}

// clearSendCap is tryClearSendCap for a call that is expected to succeed.
func (h *harness) clearSendCap(t *testing.T, c *staffeeapi.Client, id string) *staffeeapi.StaffSendCapChange {
	t.Helper()
	res, err := h.tryClearSendCap(t, c, id)
	require.NoError(t, err)
	return res
}

func (h *harness) workspaceDetail(t *testing.T, c *staffeeapi.Client, id string) *staffeeapi.StaffWorkspaceDetailResource {
	t.Helper()
	w, err := c.StaffWorkspacesGet(t.Context(), staffeeapi.StaffWorkspacesGetParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
	require.NoError(t, err)
	return w
}

// sendCapChanges are the Send cap Audit entries published so far, as JSON of their diffs.
func sendCapChanges(t *testing.T, env *testhelper.TestEnv) (entries []*events.AuditEntry, diffs []string) {
	t.Helper()
	for _, ev := range env.QueuedEvents(t, events.NameAuditEntry) {
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

	set := h.setSendCap(t, c, fixtures.AcmePublicID, 500)
	assert.True(t, set.Changed)
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(500), set.SendCap.PerDay)

	changed := h.setSendCap(t, c, fixtures.AcmePublicID, 2000)
	afterChange := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	assert.True(t, changed.Changed)
	assert.Equal(t, staffeeapi.NewNilMaxPerDay(2000), afterChange.SendCap.Value.PerDay)

	cleared := h.clearSendCap(t, c, fixtures.AcmePublicID)
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

	none := h.clearSendCap(t, c, fixtures.AcmePublicID)
	assert.False(t, none.Changed, "a Workspace with no Send cap stays as it was")

	h.setSendCap(t, c, fixtures.AcmePublicID, 500)
	same := h.setSendCap(t, c, fixtures.AcmePublicID, 500)
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

	set := h.setSendCap(t, c, fixtures.AcmePublicID, 500)
	capped := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	assert.Equal(t, staffeeapi.StaffSendCap{PerDay: staffeeapi.NewNilMaxPerDay(500), SentLast24h: sent}, set.SendCap)
	assert.Equal(t, set.SendCap, capped.SendCap.Value)

	cleared := h.clearSendCap(t, c, fixtures.AcmePublicID)
	assert.Equal(t, int32(sent), cleared.SendCap.SentLast24h)

	globex := h.workspaceDetail(t, c, fixtures.GlobexPublicID).SendCap.Value
	assert.Equal(t, int32(1), globex.SentLast24h, "each Workspace counts only its own sends")
}

func TestTheSendCapOfAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	_, err := h.trySetSendCap(t, c, "Zz9Zz9Zz", 500)
	testhelper.StaffeeRefusal(t, err, http.StatusNotFound)
	_, err = h.tryClearSendCap(t, c, "Zz9Zz9Zz")
	testhelper.StaffeeRefusal(t, err, http.StatusNotFound)
}

// A self-hosted Instance has no Send cap: its endpoints are not found whatever the License key
// (the harness's key has every feature), and the Workspace page has no Send cap to show.
func TestASelfHostedInstanceHasNoSendCapInTheConsole(t *testing.T) {
	h := newHarness(t, testhelper.WithMode(config.ModeSelfHosted))
	c, _ := h.staffSession(t)
	h.env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetSendCap(500).ExecX(t.Context())

	_, setErr := h.trySetSendCap(t, c, fixtures.AcmePublicID, 1000)
	_, clearErr := h.tryClearSendCap(t, c, fixtures.AcmePublicID)
	detail := h.workspaceDetail(t, c, fixtures.AcmePublicID)
	entries, _ := sendCapChanges(t, h.env)
	testhelper.StaffeeRefusal(t, setErr, http.StatusNotFound)
	testhelper.StaffeeRefusal(t, clearErr, http.StatusNotFound)
	assert.False(t, detail.SendCap.IsSet(), "even a stored value is not shown")
	assert.Empty(t, entries)
}
