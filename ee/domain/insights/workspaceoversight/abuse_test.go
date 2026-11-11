package workspaceoversight_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"

	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func (h *harness) setExempt(t *testing.T, c *staffeeapi.Client, id string, exempt bool) *staffeeapi.StaffExemptionChange {
	t.Helper()
	wid := staffeeapi.WorkspaceId(id)
	var change *staffeeapi.StaffExemptionChange
	var err error
	if exempt {
		change, err = c.StaffWorkspacesExempt(t.Context(), staffeeapi.StaffWorkspacesExemptParams{WorkspaceId: wid})
	} else {
		change, err = c.StaffWorkspacesUnexempt(t.Context(), staffeeapi.StaffWorkspacesUnexemptParams{WorkspaceId: wid})
	}
	require.NoError(t, err)
	return change
}

func TestAnStaffExemptsAndUnexemptsAWorkspaceAndBothAreAudited(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	exempted := h.setExempt(t, c, fixtures.AcmePublicID, true)
	again := h.setExempt(t, c, fixtures.AcmePublicID, true)
	lifted := h.setExempt(t, c, fixtures.AcmePublicID, false)
	h.env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	assert.True(t, exempted.Changed)
	assert.True(t, exempted.Workspace.Exempt)
	assert.False(t, again.Changed, "idempotent")
	assert.True(t, lifted.Changed)
	assert.False(t, lifted.Workspace.Exempt)
	entries := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.ActionIn(events.ActionWorkspaceAbuseExempt, events.ActionWorkspaceAbuseUnexempt)).AllX(t.Context())
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, auditentry.ActorKindStaff, e.ActorKind)
	}
}

func TestExemptingAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	_, err := c.StaffWorkspacesExempt(t.Context(), staffeeapi.StaffWorkspacesExemptParams{WorkspaceId: "NoSuchId"})

	testhelper.StaffeeRefusal(t, err, http.StatusNotFound)
}

func TestAnExemptWorkspaceStaysListedAndCanStillBeSuspendedByHand(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Search: staffeeapi.NewOptString(fixtures.KramericaPublicID)})
	change := h.suspend(t, c, fixtures.KramericaPublicID, "verified abuse")

	require.Len(t, page.Items, 1)
	assert.True(t, page.Items[0].Exempt)
	assert.True(t, change.Changed)
}

func TestUnsuspendingFromTheConsoleRecordsTheClearance(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	before := h.workspaceDetail(t, c, fixtures.HooliPublicID)
	require.False(t, before.Abuse.ClearedAt.Set)

	h.unsuspend(t, c, fixtures.HooliPublicID)

	after := h.workspaceDetail(t, c, fixtures.HooliPublicID)
	require.True(t, after.Abuse.ClearedAt.Set)
	assert.WithinDuration(t, time.Now(), time.Time(after.Abuse.ClearedAt.Value), time.Minute)
}

func TestTheListSurfacesWarnedAndSystemFrozenWorkspaces(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.env.DB.Workspace.UpdateOneID(fixtures.GlobexID).SetAbuseWarningNotifiedAt(time.Now()).ExecX(t.Context())

	frozen := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Review: staffeeapi.NewOptStaffWorkspaceReview(staffeeapi.StaffWorkspaceReviewSystemFrozen)})
	warned := h.listWorkspaces(t, c, staffeeapi.StaffWorkspacesListParams{Review: staffeeapi.NewOptStaffWorkspaceReview(staffeeapi.StaffWorkspaceReviewWarned)})

	assert.Equal(t, []string{fixtures.NakatomiName}, names(frozen.Items), "Hooli and Monarch were suspended by a StaffMember and Sirius from the command line, not by the system")
	assert.Equal(t, []string{"Globex"}, names(warned.Items))
	assert.True(t, warned.Items[0].Warned)
}

func TestTheDetailShowsTheDetectorsStandingAndLastNotices(t *testing.T) {
	h := newHarness(t, testhelper.WithStaffVolumeFloor(8)) // Soylent: 8 sent, 1 hard bounce, 1 complaint
	c, _ := h.staffSession(t)
	notified := time.Now().Add(-time.Hour)
	h.env.DB.Workspace.UpdateOneID(fixtures.SoylentID).SetAbuseWarningNotifiedAt(notified).ExecX(t.Context())

	detail := h.workspaceDetail(t, c, fixtures.SoylentPublicID)

	require.True(t, detail.Abuse.Warning.Set, "its rates are over the limits")
	assert.Equal(t, "mail.soylent.test", detail.Abuse.Warning.Value.Domain)
	assert.WithinDuration(t, notified, time.Time(detail.Abuse.WarningNotifiedAt.Value), time.Second)
	assert.False(t, detail.Abuse.FreezeNotifiedAt.Set)
	assert.False(t, detail.Abuse.Exempt)
}

func TestAnExemptWorkspaceHasNoWarningButKeepsItsRates(t *testing.T) {
	h := newHarness(t, testhelper.WithStaffVolumeFloor(8))
	c, _ := h.staffSession(t)
	h.setExempt(t, c, fixtures.SoylentPublicID, true)

	detail := h.workspaceDetail(t, c, fixtures.SoylentPublicID)

	assert.False(t, detail.Abuse.Warning.Set)
	assert.True(t, detail.Abuse.Exempt)
	assert.EqualValues(t, 8, detail.Deliverability.SendVolume)
}

// The detector judges the traffic of the "over the freeze limit" scenario in fixtures/events.yml
// (Vandelay, three days ago, at a floor of 20 sent), at the moment of that scenario.
func TestAWorkspaceFrozenByTheDetectorRefusesSendsOnTheAPI(t *testing.T) {
	// The pass runs at the moment of the scenario: the env's clock is three days back.
	scenario := time.Now().Add(-3 * 24 * time.Hour)
	h := newHarness(t, testhelper.WithStaffVolumeFloor(20), testhelper.WithClock(func() time.Time { return scenario }))
	summary, err := h.env.Abuse().Run(t.Context())
	require.NoError(t, err)
	require.Equal(t, []int64{fixtures.VandelayID}, summary.Frozen)
	vandelay := h.env.ExternalWithToken(t, h.env.ScopedBearerFor(t, fixtures.VandelayID, "emails:send"))

	_, err = vandelay.EmailsSend(t.Context(), &externalapi.SendTransactionalEmailInput{
		TemplateId: externalapi.EntityId(int64(fixtures.TemplateVandelayID)), Destination: "held@example.com",
	}, externalapi.EmailsSendParams{})

	held := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity, "a send is refused with a 4xx")
	assert.Equal(t, "the message was not sent: "+outboundmessages.Detail(holds.ReasonSuspended), held.Detail.Value)
}
