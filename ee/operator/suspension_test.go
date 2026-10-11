package operator_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	collectapi "github.com/mokevnin/sphericon/gen/collect"
	externalapi "github.com/mokevnin/sphericon/gen/external"
	operatorapi "github.com/mokevnin/sphericon/gen/operator"
	siteapi "github.com/mokevnin/sphericon/gen/site"
	"github.com/mokevnin/sphericon/internal/events"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/outbound"
)

func (h *harness) suspend(t *testing.T, c *operatorapi.Client, id int64, reason string) operatorapi.OperatorWorkspacesSuspendRes {
	t.Helper()
	res, err := c.OperatorWorkspacesSuspend(t.Context(), &operatorapi.OperatorSuspendInput{Reason: reason},
		operatorapi.OperatorWorkspacesSuspendParams{WorkspaceId: operatorapi.EntityId(strconv.FormatInt(id, 10))})
	require.NoError(t, err)
	return res
}

func (h *harness) unsuspend(t *testing.T, c *operatorapi.Client, id int64) operatorapi.OperatorWorkspacesUnsuspendRes {
	t.Helper()
	res, err := c.OperatorWorkspacesUnsuspend(t.Context(),
		operatorapi.OperatorWorkspacesUnsuspendParams{WorkspaceId: operatorapi.EntityId(strconv.FormatInt(id, 10))})
	require.NoError(t, err)
	return res
}

func (h *harness) sendOnAPI(t *testing.T, to string) externalapi.EmailsSendRes {
	t.Helper()
	res, err := h.env.ExternalScoped(t, "emails:send").EmailsSend(t.Context(), &externalapi.SendTransactionalEmailInput{
		TemplateId: externalapi.EntityId(strconv.Itoa(fixtures.TemplateWelcomeID)), Destination: externalapi.EmailAddress(to),
	}, externalapi.EmailsSendParams{})
	require.NoError(t, err)
	return res
}

func TestAnOperatorSuspendsAndUnsuspendsAWorkspace(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	res, ok := h.suspend(t, c, fixtures.AcmeID, "abuse report").(*operatorapi.OperatorSuspensionChange)
	require.True(t, ok)
	assert.True(t, res.Changed)
	require.True(t, res.Workspace.Suspension.IsSet())
	s := res.Workspace.Suspension.Value
	assert.Equal(t, operatorapi.OperatorSuspensionActorKindOperator, s.Actor.Kind)
	assert.Equal(t, strconv.Itoa(fixtures.OperatorEnrolledID), s.Actor.ID.Value, "the session's Operator id is the actor")
	assert.Equal(t, "abuse report", s.Reason.Value)

	res, ok = h.unsuspend(t, c, fixtures.AcmeID).(*operatorapi.OperatorSuspensionChange)
	require.True(t, ok)
	assert.True(t, res.Changed)
	assert.False(t, res.Workspace.Suspension.IsSet())
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspendingTwiceAndUnsuspendingAnActiveWorkspaceChangeNothing(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	again, ok := h.unsuspend(t, c, fixtures.AcmeID).(*operatorapi.OperatorSuspensionChange)
	require.True(t, ok)
	assert.False(t, again.Changed, "an active Workspace stays as it was")

	h.suspend(t, c, fixtures.AcmeID, "first")
	second, ok := h.suspend(t, c, fixtures.AcmeID, "second").(*operatorapi.OperatorSuspensionChange)
	require.True(t, ok)
	assert.False(t, second.Changed)
	assert.Equal(t, "first", second.Workspace.Suspension.Value.Reason.Value, "the first reason stands")
	assert.Len(t, h.env.SystemMail.Messages(), 1, "the owner is told once")
}

func TestSuspendRequiresAReason(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	res := h.suspend(t, c, fixtures.AcmeID, "   ")
	assert.IsType(t, &operatorapi.OperatorWorkspacesSuspendUnprocessableEntity{}, res)
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspendAndUnsuspendAnUnknownWorkspaceAreNotFound(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	assert.IsType(t, &operatorapi.OperatorWorkspacesSuspendNotFound{}, h.suspend(t, c, 999999, "abuse"))
	assert.IsType(t, &operatorapi.OperatorWorkspacesUnsuspendNotFound{}, h.unsuspend(t, c, 999999))
}

func TestSuspensionActionsNeedAnOperatorSession(t *testing.T) {
	h := newHarness(t)

	res, err := h.env.OperatorAnonymous(t).OperatorWorkspacesSuspend(t.Context(), &operatorapi.OperatorSuspendInput{Reason: "x"},
		operatorapi.OperatorWorkspacesSuspendParams{WorkspaceId: "1"})
	require.NoError(t, err)
	assert.IsType(t, &operatorapi.OperatorWorkspacesSuspendUnauthorized{}, res)
	assert.Nil(t, h.env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).SuspendedAt)
}

func TestSuspensionTellsTheOwnerAndKeepsTheOperatorOffTheNotice(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	h.suspend(t, c, fixtures.AcmeID, "abuse report")

	msgs := h.env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Text, "abuse report")
	assert.NotContains(t, msgs[0].Text, fixtures.OperatorEnrolledEmail)
}

// The freeze is the core one: sends on /api are held, /collect and reads keep working,
// and sending goes through again once lifted.
func TestSuspendingFreezesSendsThroughOutboundAndNothingElse(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)
	h.suspend(t, c, fixtures.AcmeID, "abuse report")

	held, ok := h.sendOnAPI(t, "held@example.com").(*externalapi.EmailsSendUnprocessableEntity)
	require.True(t, ok, "a send is refused with a 4xx")
	assert.Equal(t, outbound.HoldDetail(outbound.HoldSuspended), held.Detail.Value)
	assert.Empty(t, h.env.CustomerMail.Messages())

	collected, err := h.env.CollectAcme(t).CollectEventsCreate(t.Context(), &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, collected, "/collect keeps working")
	read, err := h.env.ExternalAnchor(t).ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListOK{}, read, "reads keep working")

	h.unsuspend(t, c, fixtures.AcmeID)
	assert.IsType(t, &externalapi.SendTransactionalEmailResponse{}, h.sendOnAPI(t, "resumed@example.com"), "sending resumes")
}

func TestTheAuditEntryShowsSphericonStaffAndNeverTheOperator(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)
	h.suspend(t, c, fixtures.AcmeID, "abuse report")
	h.env.DeliverToEE(t)

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	page, ok := res.(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "got %T", res)
	var got []siteapi.SiteAuditEntryResource
	for _, e := range page.Items {
		if e.Action == events.ActionWorkspaceSuspend {
			got = append(got, e)
		}
	}
	require.Len(t, got, 1)
	assert.Equal(t, "sphericon staff", got[0].Actor.Name.Value)
	assert.False(t, got[0].Actor.ID.IsSet() && got[0].Actor.ID.Value != "")
}
