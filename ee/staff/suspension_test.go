package staff_test

import (
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/holds"
	"github.com/getsphericon/sphericon/internal/outbound"
)

func (h *harness) suspend(t *testing.T, c *staffapi.Client, id string, reason string) staffapi.StaffWorkspacesSuspendRes {
	t.Helper()
	res, err := c.StaffWorkspacesSuspend(t.Context(), &staffapi.StaffSuspendInput{Reason: reason},
		staffapi.StaffWorkspacesSuspendParams{WorkspaceId: staffapi.WorkspaceId(id)})
	require.NoError(t, err)
	return res
}

func (h *harness) unsuspend(t *testing.T, c *staffapi.Client, id string) staffapi.StaffWorkspacesUnsuspendRes {
	t.Helper()
	res, err := c.StaffWorkspacesUnsuspend(t.Context(),
		staffapi.StaffWorkspacesUnsuspendParams{WorkspaceId: staffapi.WorkspaceId(id)})
	require.NoError(t, err)
	return res
}

func (h *harness) sendOnAPI(t *testing.T, to string) externalapi.EmailsSendRes {
	t.Helper()
	res, err := h.env.ExternalScoped(t, "emails:send").EmailsSend(t.Context(), &externalapi.SendTransactionalEmailInput{
		TemplateId: externalapi.EntityId(int64(fixtures.TemplateWelcomeID)), Destination: externalapi.EmailAddress(to),
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	return res
}

func TestAnStaffSuspendsAndUnsuspendsAWorkspace(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res, ok := h.suspend(t, c, fixtures.AcmePublicID, "abuse report").(*staffapi.StaffSuspensionChange)
	require.True(t, ok)
	assert.True(t, res.Changed)
	require.True(t, res.Workspace.Suspension.IsSet())
	s := res.Workspace.Suspension.Value
	assert.Equal(t, staffapi.StaffSuspensionActorKindStaff, s.Actor.Kind)
	assert.Equal(t, strconv.Itoa(fixtures.StaffEnrolledID), s.Actor.ID.Value, "the session's StaffMember id is the actor")
	assert.Equal(t, "abuse report", s.Reason.Value)

	res, ok = h.unsuspend(t, c, fixtures.AcmePublicID).(*staffapi.StaffSuspensionChange)
	require.True(t, ok)
	assert.True(t, res.Changed)
	assert.False(t, res.Workspace.Suspension.IsSet())
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspendingTwiceAndUnsuspendingAnActiveWorkspaceChangeNothing(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	again, ok := h.unsuspend(t, c, fixtures.AcmePublicID).(*staffapi.StaffSuspensionChange)
	require.True(t, ok)
	assert.False(t, again.Changed, "an active Workspace stays as it was")

	h.suspend(t, c, fixtures.AcmePublicID, "first")
	second, ok := h.suspend(t, c, fixtures.AcmePublicID, "second").(*staffapi.StaffSuspensionChange)
	require.True(t, ok)
	assert.False(t, second.Changed)
	assert.Equal(t, "first", second.Workspace.Suspension.Value.Reason.Value, "the first reason stands")
	assert.Len(t, h.env.SystemMail.Messages(), 1, "the owner is told once")
}

func TestSuspendRequiresAReason(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res := h.suspend(t, c, fixtures.AcmePublicID, "   ")
	assert.IsType(t, &staffapi.StaffWorkspacesSuspendUnprocessableEntity{}, res)
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspendAndUnsuspendAnUnknownWorkspaceAreNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	assert.IsType(t, &staffapi.StaffWorkspacesSuspendNotFound{}, h.suspend(t, c, "NoSuchId", "abuse"))
	assert.IsType(t, &staffapi.StaffWorkspacesUnsuspendNotFound{}, h.unsuspend(t, c, "NoSuchId"))
}

func TestSuspensionActionsNeedAnStaffSession(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.StaffAnonymous(t).StaffWorkspacesSuspend(t.Context(), &staffapi.StaffSuspendInput{Reason: "x"},
		staffapi.StaffWorkspacesSuspendParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &staffapi.StaffWorkspacesSuspendUnauthorized{}, res)
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspensionTellsTheOwnerAndKeepsTheStaffOffTheNotice(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")

	msgs := h.env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Text, "abuse report")
	assert.NotContains(t, msgs[0].Text, fixtures.StaffEnrolledEmail)
}

// The freeze is the core one: sends on /api are held, /collect and reads keep working,
// and sending goes through again once lifted.
func TestSuspendingFreezesSendsThroughOutboundAndNothingElse(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")

	held, ok := h.sendOnAPI(t, "held@example.com").(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok, "a send is refused with a 4xx")
	assert.Equal(t, outbound.Detail(holds.ReasonSuspended), held.Detail.Value)
	assert.Empty(t, h.env.CustomerMail.Messages())

	collected, err := h.env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, collected, "/collect keeps working")
	read, err := h.env.ExternalAnchor(t).ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListOK{}, read, "reads keep working")

	h.unsuspend(t, c, fixtures.AcmePublicID)
	assert.IsType(t, &externalapi.SendTransactionalEmailResponse{}, h.sendOnAPI(t, "resumed@example.com"), "sending resumes")
}

func TestTheAuditEntryShowsSphericonStaffAndNeverTheStaffMember(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	h.env.DeliverToEE(t)

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	page, ok := res.(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "got %T", res)
	got := lo.Filter(page.Items, func(e siteapi.SiteAuditEntryResource, _ int) bool { return e.Action == events.ActionWorkspaceSuspend })
	require.Len(t, got, 1)
	assert.Equal(t, "Sphericon staff", got[0].Actor.Name.Value)
	assert.False(t, got[0].Actor.ID.IsSet() && got[0].Actor.ID.Value != "")
}
