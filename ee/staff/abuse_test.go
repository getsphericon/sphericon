package staff_test

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	staffapi "github.com/getsphericon/sphericon/gen/staff"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func (h *harness) setExempt(t *testing.T, c *staffapi.Client, id int64, exempt bool) *staffapi.StaffExemptionChange {
	t.Helper()
	wid := staffapi.EntityId(strconv.FormatInt(id, 10))
	var res any
	var err error
	if exempt {
		res, err = c.StaffWorkspacesExempt(t.Context(), staffapi.StaffWorkspacesExemptParams{WorkspaceId: wid})
	} else {
		res, err = c.StaffWorkspacesUnexempt(t.Context(), staffapi.StaffWorkspacesUnexemptParams{WorkspaceId: wid})
	}
	require.NoError(t, err)
	change, ok := res.(*staffapi.StaffExemptionChange)
	require.Truef(t, ok, "got %T", res)
	return change
}

func TestAnStaffExemptsAndUnexemptsAWorkspaceAndBothAreAudited(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	exempted := h.setExempt(t, c, fixtures.AcmeID, true)
	again := h.setExempt(t, c, fixtures.AcmeID, true)
	lifted := h.setExempt(t, c, fixtures.AcmeID, false)
	h.env.DeliverToEE(t)

	assert.True(t, exempted.Changed)
	assert.True(t, exempted.Workspace.Exempt)
	assert.False(t, again.Changed, "idempotent")
	assert.True(t, lifted.Changed)
	assert.False(t, lifted.Workspace.Exempt)
	entries := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.ActionIn(events.ActionWorkspaceAbuseExempt, events.ActionWorkspaceAbuseUnexempt)).AllX(t.Context())
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, string(events.ActorStaff), string(e.ActorKind))
	}
}

func TestExemptingAnUnknownWorkspaceIsNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res, err := c.StaffWorkspacesExempt(t.Context(), staffapi.StaffWorkspacesExemptParams{WorkspaceId: "999999"})

	require.NoError(t, err)
	assert.IsType(t, &staffapi.StaffWorkspacesExemptNotFound{}, res)
}

func TestAnExemptWorkspaceStaysListedAndCanStillBeSuspendedByHand(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Slug: staffapi.NewOptString(fixtures.KramericaSlug)})
	res := h.suspend(t, c, fixtures.KramericaID, "verified abuse")

	require.Len(t, page.Items, 1)
	assert.True(t, page.Items[0].Exempt)
	change, ok := res.(*staffapi.StaffSuspensionChange)
	require.True(t, ok)
	assert.True(t, change.Changed)
}

func TestUnsuspendingFromTheConsoleRecordsTheClearance(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	before := h.getWorkspace(t, c, strconv.Itoa(fixtures.HooliID)).(*staffapi.StaffWorkspaceDetailResource)
	require.False(t, before.Abuse.ClearedAt.Set)

	h.unsuspend(t, c, fixtures.HooliID)

	after := h.getWorkspace(t, c, strconv.Itoa(fixtures.HooliID)).(*staffapi.StaffWorkspaceDetailResource)
	require.True(t, after.Abuse.ClearedAt.Set)
	assert.WithinDuration(t, time.Now(), time.Time(after.Abuse.ClearedAt.Value), time.Minute)
}

func TestTheListSurfacesWarnedAndSystemFrozenWorkspaces(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	_, err := suspension.SuspendWorkspace(t.Context(), h.env.Bus, fixtures.VandelayID, suspension.System, "complaint rate")
	require.NoError(t, err)
	h.env.DB.Workspace.UpdateOneID(fixtures.GlobexID).SetAbuseWarningNotifiedAt(time.Now()).ExecX(t.Context())

	frozen := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Review: staffapi.NewOptStaffWorkspaceReview(staffapi.StaffWorkspaceReviewSystemFrozen)})
	warned := h.listWorkspaces(t, c, staffapi.StaffWorkspacesListParams{Review: staffapi.NewOptStaffWorkspaceReview(staffapi.StaffWorkspaceReviewWarned)})

	assert.Equal(t, []string{"vandelay"}, slugs(frozen.Items), "Hooli was suspended by a StaffMember, not the system")
	assert.Equal(t, []string{"globex"}, slugs(warned.Items))
	assert.True(t, warned.Items[0].Warned)
}

func TestTheDetailShowsTheDetectorsStandingAndLastNotices(t *testing.T) {
	h := newHarness(t, testhelper.WithStaffVolumeFloor(8)) // Soylent: 8 sent, 1 hard bounce, 1 complaint
	c, _ := h.staffSession(t)
	notified := time.Now().Add(-time.Hour)
	h.env.DB.Workspace.UpdateOneID(fixtures.SoylentID).SetAbuseWarningNotifiedAt(notified).ExecX(t.Context())

	detail := h.getWorkspace(t, c, strconv.Itoa(fixtures.SoylentID)).(*staffapi.StaffWorkspaceDetailResource)

	require.True(t, detail.Abuse.Warning.Set, "its rates are over the limits")
	assert.Equal(t, "mail.soylent.test", detail.Abuse.Warning.Value.Domain)
	assert.WithinDuration(t, notified, time.Time(detail.Abuse.WarningNotifiedAt.Value), time.Second)
	assert.False(t, detail.Abuse.FreezeNotifiedAt.Set)
	assert.False(t, detail.Abuse.Exempt)
}

func TestAnExemptWorkspaceHasNoWarningButKeepsItsRates(t *testing.T) {
	h := newHarness(t, testhelper.WithStaffVolumeFloor(8))
	c, _ := h.staffSession(t)
	h.setExempt(t, c, fixtures.SoylentID, true)

	detail := h.getWorkspace(t, c, strconv.Itoa(fixtures.SoylentID)).(*staffapi.StaffWorkspaceDetailResource)

	assert.False(t, detail.Abuse.Warning.Set)
	assert.True(t, detail.Abuse.Exempt)
	assert.EqualValues(t, 8, detail.Deliverability.SendVolume)
}

func TestAWorkspaceFrozenByTheDetectorRefusesSendsOnTheAPI(t *testing.T) {
	h := newHarness(t)
	now := time.Now()
	s := h.env.DB.Scoped(fixtures.AcmeID)
	var creates []*ent.EventScopedCreate
	for i := range 2000 {
		creates = append(creates, s.Event().Create().SetEmail(fmt.Sprintf("s%d@example.com", i)).
			SetAction(events.NameEmailSent).SetProperties(map[string]any{"sendingDomain": fixtures.SendingDomainVerifiedDomain}).SetOccurredAt(now.Add(-time.Hour)))
	}
	for i := range 20 {
		creates = append(creates, s.Event().Create().SetEmail(fmt.Sprintf("c%d@example.com", i)).
			SetAction(events.NameEmailComplained).SetProperties(map[string]any{"sendingDomain": fixtures.SendingDomainVerifiedDomain}).SetOccurredAt(now.Add(-time.Hour)))
	}
	require.NoError(t, s.Event().CreateBulk(creates...).Exec(t.Context()))

	summary, err := h.env.Abuse().Run(t.Context(), now)
	require.NoError(t, err)
	require.Equal(t, []int64{fixtures.AcmeID}, summary.Frozen)

	held, ok := h.sendOnAPI(t, "held@example.com").(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok, "a send is refused with a 4xx")
	assert.Equal(t, outbound.HoldDetail(outbound.HoldSuspended), held.Detail.Value)
}
