package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteSendingDomainsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	known := idStr(fixtures.SendingDomainUnverifiedID)
	missing := idStr(99999)
	foreignID := idStr(fixtures.SendingDomainGlobexID)

	// Create: foreign workspaceID, invalid domain, duplicate.
	r1, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "new.example.com"}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsCreateNotFound{}, r1)
	r2, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "localhost"}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsCreateUnprocessableEntity{}, r2)
	r2b, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: "a..example.com"}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsCreateUnprocessableEntity{}, r2b, "an empty label is invalid")
	r3, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{Domain: fixtures.SendingDomainVerifiedDomain}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsCreateConflict{}, r3, "a duplicate domain is a conflict")

	// Create with an explicit selector persists it.
	r4, err := c.SiteSendingDomainsCreate(ctx, &siteapi.SiteCreateSendingDomainInput{
		Domain: " Mail.Example.COM. ", DkimSelector: siteapi.NewOptString(" sel1 "),
	}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	created, ok := r4.(*siteapi.SiteSendingDomainResource)
	require.Truef(t, ok, "got %T", r4)
	stored, err := env.DB.SendingDomain.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "mail.example.com", stored.Domain)
	assert.Equal(t, "sel1", stored.DkimSelector)

	// Get.
	g1, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsGetNotFound{}, g1)
	g3, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: acme, ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsGetNotFound{}, g3)
	g4, err := c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainResource{}, g4)

	// Verify.
	v1, err := c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsVerifyNotFound{}, v1)
	v3, err := c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: acme, ID: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsVerifyNotFound{}, v3)
	v4, err := c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: acme, ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsVerifyNotFound{}, v4)

	// Delete.
	d1, err := c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: foreign, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNotFound{}, d1)
	d3, err := c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: acme, ID: foreignID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNotFound{}, d3)
	d4, err := c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSendingDomainsDeleteNoContent{}, d4)
	gone, err := env.DB.SendingDomain.Query().Where(sendingdomain.ID(fixtures.SendingDomainUnverifiedID)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, gone)
	kept, err := env.DB.SendingDomain.Query().Where(sendingdomain.ID(fixtures.SendingDomainGlobexID)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, kept)
}

func TestSiteTagsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	alice := idStr(fixtures.ContactAliceID)
	missing := idStr(99999)

	// Listing a contact's tags.
	l1, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: foreign, ContactId: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsListForContactNotFound{}, l1)
	l3, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsListForContactNotFound{}, l3)
	l4, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsListForContactOK{}, l4)

	// Applying.
	a1, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "t"}, siteapi.SiteTagsApplyParams{WorkspaceId: foreign, ContactId: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsApplyNotFound{}, a1)
	a3, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "t"}, siteapi.SiteTagsApplyParams{WorkspaceId: acme, ContactId: missing})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsApplyNotFound{}, a3)
	a4, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "   "}, siteapi.SiteTagsApplyParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsApplyUnprocessableEntity{}, a4)

	// Applying then removing a tag round-trips through the contact's tag list.
	applied, err := c.SiteTagsApply(ctx, &siteapi.SiteApplyTagInput{Name: "round-trip"}, siteapi.SiteTagsApplyParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteTagResource{}, applied)
	listed, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	assert.Contains(t, siteTagNames(listed.(*siteapi.SiteTagsListForContactOK).Items), "round-trip")

	// Removing.
	rm1, err := c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: foreign, ContactId: alice, Name: "round-trip"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsRemoveNotFound{}, rm1)
	rm3, err := c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: acme, ContactId: missing, Name: "round-trip"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsRemoveNotFound{}, rm3)
	rm4, err := c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: acme, ContactId: alice, Name: "round-trip"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsRemoveNoContent{}, rm4)
	after, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	assert.NotContains(t, siteTagNames(after.(*siteapi.SiteTagsListForContactOK).Items), "round-trip")

	// The workspace list pages and is workspace-scoped.
	p, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: acme, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(1)})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(p.(*siteapi.SiteTagsListOK).Items), 1)
	nf, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: foreign})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsListNotFound{}, nf)
}
