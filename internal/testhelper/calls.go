package testhelper

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// DemoteVehementAdmin makes Vehement's admin (VehementAdminMembership) a member, as its Owner Val
// does in the console: the request after it reads the lower Role.
func (env *TestEnv) DemoteVehementAdmin(t *testing.T) {
	t.Helper()
	_, err := env.SiteActor(t, fixtures.VehementOwnerValEmail).SiteMembershipsUpdate(t.Context(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleMember},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.VehementPublicID, ID: siteapi.EntityId(fixtures.VehementAdminMembershipID)})
	require.NoError(t, err)
}

// ListContactsWith is the error /api answers a contacts list on the bearer with: nil when it lists.
func (env *TestEnv) ListContactsWith(t *testing.T, bearer string) error {
	t.Helper()
	_, err := env.ExternalWithToken(t, bearer).ContactsList(t.Context(), externalapi.ContactsListParams{})
	return err
}

// IdentifyVisitor identifies Acme's Visitor by the email through the tracker's collect key.
func (env *TestEnv) IdentifyVisitor(t *testing.T, visitorID, email string) {
	t.Helper()
	_, err := env.CollectAcme(t).CollectIdentifyCreate(t.Context(), &collectapi.CollectIdentifyInput{
		VisitorId: visitorID, Email: collectapi.NewOptNilEmailAddress(collectapi.EmailAddress(email)),
	})
	require.NoError(t, err)
}

// AttentionItemAbout is the item of the Needs attention report of that kind about the subject id.
func AttentionItemAbout(items []siteapi.SiteAttentionItem, kind siteapi.SiteAttentionKind, id int64) (siteapi.SiteAttentionItem, bool) {
	return lo.Find(items, func(i siteapi.SiteAttentionItem) bool {
		return i.Kind == kind && i.Subject.Value.ID == siteapi.EntityId(id)
	})
}
