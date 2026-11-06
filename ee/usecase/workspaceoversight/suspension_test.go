package workspaceoversight_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func (h *harness) trySuspend(t *testing.T, c *staffeeapi.Client, id, reason string) (*staffeeapi.StaffSuspensionChange, error) {
	t.Helper()
	return c.StaffWorkspacesSuspend(t.Context(), &staffeeapi.StaffSuspendInput{Reason: reason},
		staffeeapi.StaffWorkspacesSuspendParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
}

func (h *harness) tryUnsuspend(t *testing.T, c *staffeeapi.Client, id string) (*staffeeapi.StaffSuspensionChange, error) {
	t.Helper()
	return c.StaffWorkspacesUnsuspend(t.Context(),
		staffeeapi.StaffWorkspacesUnsuspendParams{WorkspaceId: staffeeapi.WorkspaceId(id)})
}

// suspend is trySuspend for a call that is expected to succeed.
func (h *harness) suspend(t *testing.T, c *staffeeapi.Client, id, reason string) *staffeeapi.StaffSuspensionChange {
	t.Helper()
	res, err := h.trySuspend(t, c, id, reason)
	require.NoError(t, err)
	return res
}

// unsuspend is tryUnsuspend for a call that is expected to succeed.
func (h *harness) unsuspend(t *testing.T, c *staffeeapi.Client, id string) *staffeeapi.StaffSuspensionChange {
	t.Helper()
	res, err := h.tryUnsuspend(t, c, id)
	require.NoError(t, err)
	return res
}

// trySendOnAPI sends the welcome template to on /api.
func (h *harness) trySendOnAPI(t *testing.T, to string) (*externalapi.SendTransactionalEmailResponse, error) {
	t.Helper()
	return h.env.ExternalScoped(t, "emails:send").EmailsSend(t.Context(), &externalapi.SendTransactionalEmailInput{
		TemplateId: externalapi.EntityId(int64(fixtures.TemplateWelcomeID)), Destination: externalapi.EmailAddress(to),
	}, externalapi.EmailsSendParams{})
}

// sendOnAPI is trySendOnAPI for a send that is expected to go through.
func (h *harness) sendOnAPI(t *testing.T, to string) *externalapi.SendTransactionalEmailResponse {
	t.Helper()
	res, err := h.trySendOnAPI(t, to)
	require.NoError(t, err)
	return res
}

// refusedSendOnAPI is trySendOnAPI for a send that is expected to be refused with a 4xx.
func (h *harness) refusedSendOnAPI(t *testing.T, to string, msgAndArgs ...any) externalapi.ProblemDetails {
	t.Helper()
	_, err := h.trySendOnAPI(t, to)
	return testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity, msgAndArgs...)
}

func TestAnStaffSuspendsAndUnsuspendsAWorkspace(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	res := h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	assert.True(t, res.Changed)
	require.True(t, res.Workspace.Suspension.IsSet())
	s := res.Workspace.Suspension.Value
	assert.Equal(t, staffeeapi.StaffSuspensionActorKindStaff, s.Actor.Kind)
	assert.Equal(t, strconv.Itoa(fixtures.StaffEnrolledID), s.Actor.ID.Value, "the session's StaffMember id is the actor")
	assert.Equal(t, "abuse report", s.Reason.Value)

	res = h.unsuspend(t, c, fixtures.AcmePublicID)
	assert.True(t, res.Changed)
	assert.False(t, res.Workspace.Suspension.IsSet())
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspendingTwiceAndUnsuspendingAnActiveWorkspaceChangeNothing(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	again := h.unsuspend(t, c, fixtures.AcmePublicID)
	assert.False(t, again.Changed, "an active Workspace stays as it was")

	h.suspend(t, c, fixtures.AcmePublicID, "first")
	second := h.suspend(t, c, fixtures.AcmePublicID, "second")
	assert.False(t, second.Changed)
	assert.Equal(t, "first", second.Workspace.Suspension.Value.Reason.Value, "the first reason stands")
	assert.Len(t, h.env.SystemMail.Messages(), 1, "the owner is told once")
}

func TestSuspendRequiresAReason(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	_, err := h.trySuspend(t, c, fixtures.AcmePublicID, "   ")
	testhelper.StaffeeRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspendAndUnsuspendAnUnknownWorkspaceAreNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	_, err := h.trySuspend(t, c, "NoSuchId", "abuse")
	testhelper.StaffeeRefusal(t, err, http.StatusNotFound)
	_, err = h.tryUnsuspend(t, c, "NoSuchId")
	testhelper.StaffeeRefusal(t, err, http.StatusNotFound)
}

func TestSuspensionActionsNeedAnStaffSession(t *testing.T) {
	h := newHarness(t)

	_, err := h.env.StaffEnterpriseWithToken(t, "").StaffWorkspacesSuspend(t.Context(), &staffeeapi.StaffSuspendInput{Reason: "x"},
		staffeeapi.StaffWorkspacesSuspendParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.StaffeeRefusal(t, err, http.StatusUnauthorized)
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

	held := h.refusedSendOnAPI(t, "held@example.com", "a send is refused with a 4xx")
	assert.Equal(t, "the message was not sent: "+outboundmessages.Detail(holds.ReasonSuspended), held.Detail.Value)
	assert.Empty(t, h.env.CustomerMail.Messages())

	err := h.env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{})
	require.NoError(t, err, "/collect keeps working")
	_, err = h.env.ExternalAnchor(t).ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, err)

	h.unsuspend(t, c, fixtures.AcmePublicID)
	h.sendOnAPI(t, "resumed@example.com")
}

func TestTheAuditEntryShowsSphericonStaffAndNeverTheStaffMember(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	h.env.WorkEditionJobs(t)

	page, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	got := lo.Filter(page.Items, func(e siteapi.SiteAuditEntryResource, _ int) bool { return e.Action == events.ActionWorkspaceSuspend })
	require.Len(t, got, 1)
	assert.Equal(t, "Sphericon staff", got[0].Actor.Name.Value)
	assert.False(t, got[0].Actor.ID.IsSet() && got[0].Actor.ID.Value != "")
}
