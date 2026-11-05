package site_test

import (
	"context"
	"net/http"
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
	_, err := c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: "new.example.com"}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: "localhost"}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	_, err = c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: "a..example.com"}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, "an empty label is invalid")
	_, err = c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{Domain: fixtures.SendingDomainVerifiedDomain}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	testhelper.SiteRefusal(t, err, http.StatusConflict, "a duplicate domain is a conflict")

	// Create with an explicit selector persists it.
	created, err := c.SiteSendingDomainsCreate(ctx, &siteapi.CreateSendingDomainInput{
		Domain: " Mail.Example.COM. ", DkimSelector: siteapi.NewOptString(" sel1 "),
	}, siteapi.SiteSendingDomainsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	stored, err := env.DB.SendingDomain.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "mail.example.com", stored.Domain)
	assert.Equal(t, "sel1", stored.DkimSelector)

	// Get.
	_, err = c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: acme, ID: foreignID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSendingDomainsGet(ctx, siteapi.SiteSendingDomainsGetParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)

	// Verify.
	_, err = c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: acme, ID: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteSendingDomainsVerify(ctx, siteapi.SiteSendingDomainsVerifyParams{WorkspaceId: acme, ID: foreignID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	// Archive and unarchive.
	err = c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: foreign, ID: known})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: acme, ID: foreignID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteSendingDomainsDelete(ctx, siteapi.SiteSendingDomainsDeleteParams{WorkspaceId: acme, ID: known})
	require.NoError(t, err)
	archived, err := env.DB.SendingDomain.Query().Where(sendingdomain.ID(fixtures.SendingDomainUnverifiedID), sendingdomain.DiscardedAtNotNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, archived)
	untouched, err := env.DB.SendingDomain.Query().Where(sendingdomain.ID(fixtures.SendingDomainGlobexID), sendingdomain.DiscardedAtIsNil()).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, untouched)
}

func TestSiteTagsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)
	alice := idStr(fixtures.ContactAliceID)
	missing := idStr(99999)

	// Listing a contact's tags.
	_, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: foreign, ContactId: alice})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)

	// Applying.
	_, err = c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "t"}, siteapi.SiteTagsApplyParams{WorkspaceId: foreign, ContactId: alice})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "t"}, siteapi.SiteTagsApplyParams{WorkspaceId: acme, ContactId: missing})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	_, err = c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "   "}, siteapi.SiteTagsApplyParams{WorkspaceId: acme, ContactId: alice})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)

	// Applying then removing a tag round-trips through the contact's tag list.
	_, err = c.SiteTagsApply(ctx, &siteapi.ApplyTagInput{Name: "round-trip"}, siteapi.SiteTagsApplyParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	listed, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	assert.Contains(t, siteTagNames(listed.Items), "round-trip")

	// Removing.
	err = c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: foreign, ContactId: alice, Name: "round-trip"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: acme, ContactId: missing, Name: "round-trip"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
	err = c.SiteTagsRemove(ctx, siteapi.SiteTagsRemoveParams{WorkspaceId: acme, ContactId: alice, Name: "round-trip"})
	require.NoError(t, err)
	after, err := c.SiteTagsListForContact(ctx, siteapi.SiteTagsListForContactParams{WorkspaceId: acme, ContactId: alice})
	require.NoError(t, err)
	assert.NotContains(t, siteTagNames(after.Items), "round-trip")

	// The workspace list pages and is workspace-scoped.
	p, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: acme, Page: siteapi.NewOptInt32(1), PageSize: siteapi.NewOptInt32(1)})
	require.NoError(t, err)
	assert.LessOrEqual(t, len(p.Items), 1)
	_, err = c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: foreign})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}
