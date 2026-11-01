package site_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

type facetActor struct {
	Kind siteapi.SiteAuditActorKind
	ID   string
	Name string
}

func auditFacets(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) siteapi.SiteAuditFacetsRes {
	t.Helper()
	res, err := c.SiteAuditFacets(context.Background(), siteapi.SiteAuditFacetsParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func facetActors(facets *siteapi.SiteAuditFacets) []facetActor {
	return lo.Map(facets.Actors, func(a siteapi.SiteAuditActor, _ int) facetActor {
		return facetActor{Kind: a.Kind, ID: a.ID.Or(""), Name: a.Name.Or("")}
	})
}

// Globex's fixture log: Jane (user 2) under two names, an unnamed API token and the system.
func TestAuditFacetsOfferWhatOccursInTheLog(t *testing.T) {
	env := testhelper.Setup(t)
	res := auditFacets(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID)
	facets, ok := res.(*siteapi.SiteAuditFacets)
	require.Truef(t, ok, "facets answered %T", res)

	assert.Equal(t, []string{
		"integration.delete", "integration.update", "membership.update", "tag.create", "webhook_endpoint.create",
	}, facets.Actions)
	assert.Equal(t, []string{"integration", "membership", "tag", "webhook_endpoint"}, facets.TargetTypes)
	assert.Equal(t, []facetActor{
		{Kind: siteapi.SiteAuditActorKindAPIToken, ID: "77"},
		{Kind: siteapi.SiteAuditActorKindSystem},
		{Kind: siteapi.SiteAuditActorKindUser, ID: "2", Name: `=HYPERLINK("http://evil")`},
	}, facetActors(facets), "one entry per actor, named by its newest entry")
}

// Initech's log holds an entry by a platform Staff member: the facets name no staff identity.
func TestAuditFacetsNeverExposeAStaffIdentity(t *testing.T) {
	env := testhelper.Setup(t)
	res := auditFacets(t, env.SiteActor(t, fixtures.InitechOwnerOlgaEmail), fixtures.InitechPublicID)
	facets, ok := res.(*siteapi.SiteAuditFacets)
	require.Truef(t, ok, "facets answered %T", res)

	assert.Equal(t, []facetActor{
		{Kind: siteapi.SiteAuditActorKindStaff, Name: "Sphericon staff"},
		{Kind: siteapi.SiteAuditActorKindUser, ID: "9", Name: "Peter"},
	}, facetActors(facets))
}

func TestAuditFacetsAreOwnerAndAdminOnly(t *testing.T) {
	env := testhelper.Setup(t)

	member := auditFacets(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID)
	assert.IsType(t, &siteapi.SiteAuditFacetsForbidden{}, member, "a member is refused")

	cross := auditFacets(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.GlobexPublicID)
	assert.IsType(t, &siteapi.SiteAuditFacetsNotFound{}, cross, "John cannot address Globex's log")
}
