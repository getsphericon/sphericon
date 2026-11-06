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
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
)

// The stored Audit row keeps the real Staff member id (the platform's own trail); only the
// customer's view of it says "Sphericon staff".
func TestTheStoredAuditRowsKeepTheStaffIdWhileTheCustomerSeesStaff(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)
	h.suspend(t, c, fixtures.AcmePublicID, "abuse report")
	h.unsuspend(t, c, fixtures.AcmePublicID)
	h.env.WorkEditionJobs(t)

	stored := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().
		Where(auditentry.ActionIn(events.ActionWorkspaceSuspend, events.ActionWorkspaceUnsuspend)).AllX(t.Context())
	require.Len(t, stored, 2)
	for _, e := range stored {
		require.NotNil(t, e.ActorID, e.Action)
		assert.Equal(t, strconv.Itoa(fixtures.StaffEnrolledID), *e.ActorID, e.Action)
	}

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	page := res
	suspensions := lo.Filter(page.Items, func(e siteapi.SiteAuditEntryResource, _ int) bool {
		return e.Action == events.ActionWorkspaceSuspend || e.Action == events.ActionWorkspaceUnsuspend
	})
	for _, e := range suspensions {
		assert.Equal(t, "Sphericon staff", e.Actor.Name.Value, e.Action)
		assert.False(t, e.Actor.ID.IsSet() && e.Actor.ID.Value != "", e.Action)
	}
	assert.Len(t, suspensions, 2)
}

func TestTheConsoleShowsSystemAndCLISuspensionsWithoutAnActorId(t *testing.T) {
	for name, tc := range map[string]struct {
		publicID string
		kind     staffeeapi.StaffSuspensionActorKind
	}{
		"system": {fixtures.NakatomiPublicID, staffeeapi.StaffSuspensionActorKindSystem},
		"cli":    {fixtures.SiriusPublicID, staffeeapi.StaffSuspensionActorKindCli},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			c, _ := h.staffSession(t)

			ws, err := h.getWorkspace(t, c, tc.publicID)
			require.NoError(t, err)
			require.True(t, ws.Suspension.Set)
			assert.Equal(t, tc.kind, ws.Suspension.Value.Actor.Kind)
			assert.False(t, ws.Suspension.Value.Actor.ID.IsSet() && !ws.Suspension.Value.Actor.ID.IsNull(), "no actor id")
			assert.Equal(t, "complaint rate", ws.Suspension.Value.Reason.Value)
		})
	}
}

func TestTheOwnerNoticeNamesSystemOrSphericonStaffAsTheActor(t *testing.T) {
	for name, tc := range map[string]struct {
		id  int64
		key i18n.ID
	}{
		"system": {fixtures.NakatomiID, i18n.EmailWorkspaceSuspendedActorSystem},
		"cli":    {fixtures.SiriusID, i18n.EmailWorkspaceSuspendedActorStaff},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)

			require.NoError(t, suspension.NotifyOwners(t.Context(), h.env.DB, memberships.OwnerNotifier(h.env.DB, h.env.SystemMail), "", tc.id))

			msgs := h.env.SystemMail.Messages()
			require.Len(t, msgs, 1)
			assert.Contains(t, msgs[0].Text, i18n.T(tc.key, nil))
		})
	}
}
