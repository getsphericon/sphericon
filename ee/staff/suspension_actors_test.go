package staff_test

import (
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/auditentry"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/suspension"
)

// The stored Audit row keeps the real Staff member id (the platform's own trail); only the
// customer's view of it says "Sphericon staff".
func TestTheStoredAuditRowsKeepTheStaffIdWhileTheCustomerSeesStaff(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	h.unsuspend(t, c, fixtures.AcmePublicID)
	h.env.DeliverToEE(t)

	stored := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.ActionIn(events.ActionWorkspaceSuspend, events.ActionWorkspaceUnsuspend)).AllX(t.Context())
	require.Len(t, stored, 2)
	for _, e := range stored {
		require.NotNil(t, e.ActorID, e.Action)
		assert.Equal(t, strconv.Itoa(fixtures.StaffEnrolledID), *e.ActorID, e.Action)
	}

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	page, ok := res.(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "got %T", res)
	suspensions := lo.Filter(page.Items, func(e siteapi.SiteAuditEntryResource, _ int) bool {
		return e.Action == events.ActionWorkspaceSuspend || e.Action == events.ActionWorkspaceUnsuspend
	})
	for _, e := range suspensions {
		assert.Equal(t, "Sphericon staff", e.Actor.Name.Value, e.Action)
		assert.False(t, e.Actor.ID.IsSet() && e.Actor.ID.Value != "", e.Action)
	}
	assert.Len(t, suspensions, 2)
}

// suspendAs suspends Acme through the core mechanism as a non-Staff member actor, which the
// console must then render and the owner be told about.
func (h *harness) suspendAs(t *testing.T, by suspension.Actor) {
	t.Helper()
	_, err := suspension.SuspendWorkspace(t.Context(), h.env.Bus, fixtures.AcmeID, by, "complaint rate")
	require.NoError(t, err)
}

func TestTheConsoleShowsSystemAndCLISuspensionsWithoutAnActorId(t *testing.T) {
	for name, tc := range map[string]struct {
		by   suspension.Actor
		kind staffeeapi.StaffSuspensionActorKind
	}{
		"system": {suspension.System, staffeeapi.StaffSuspensionActorKindSystem},
		"cli":    {suspension.CLI, staffeeapi.StaffSuspensionActorKindCli},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			c, _ := h.staffSession(t)
			h.suspendAs(t, tc.by)

			acme, ok := h.getWorkspace(t, c, fixtures.AcmePublicID).(*staffeeapi.StaffWorkspaceDetailResource)
			require.True(t, ok)
			require.True(t, acme.Suspension.Set)
			assert.Equal(t, tc.kind, acme.Suspension.Value.Actor.Kind)
			assert.False(t, acme.Suspension.Value.Actor.ID.IsSet() && !acme.Suspension.Value.Actor.ID.IsNull(), "no actor id")
			assert.Equal(t, "complaint rate", acme.Suspension.Value.Reason.Value)
		})
	}
}

func TestTheOwnerNoticeNamesSystemOrSphericonStaffAsTheActor(t *testing.T) {
	for name, tc := range map[string]struct {
		by  suspension.Actor
		key i18n.ID
	}{
		"system": {suspension.System, i18n.EmailWorkspaceSuspendedActorSystem},
		"cli":    {suspension.CLI, i18n.EmailWorkspaceSuspendedActorStaff},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.suspendAs(t, tc.by)

			require.NoError(t, suspension.NotifyOwners(t.Context(), h.env.DB, accounts.OwnerNotifier(h.env.DB, h.env.SystemMail), "", fixtures.AcmeID))

			msgs := h.env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0].Text, i18n.T(tc.key, nil))
		})
	}
}
