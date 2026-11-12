package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Manual suppression CRUD: create (normalized + idempotent), list, delete.
func TestSiteSuppressionsCRUD(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	// Create normalizes the destination and defaults the reason to manual.
	created, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "Blocked@Example.com"},
		siteapi.SiteSuppressionsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	res := created
	assert.Equal(t, "blocked@example.com", res.Destination)
	assert.Equal(t, siteapi.SiteSuppressionReasonManual, res.Reason)

	// Suppressing the same destination again is idempotent (no duplicate).
	again, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "blocked@example.com"},
		siteapi.SiteSuppressionsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	againRes := again
	assert.Equal(t, res.ID, againRes.ID, "idempotent: same entry returned")

	// The row is persisted under the workspace (selected from the DB by its key).
	exists, err := env.DB.Suppression.Query().
		Where(suppression.WorkspaceID(fixtures.AcmeID), suppression.Destination("blocked@example.com")).
		Exist(ctx)
	require.NoError(t, err)
	assert.True(t, exists, "created suppression is persisted")

	// Lifting keeps the row, stamped, and the destination is no longer refused.
	err = c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: workspaceID, ID: res.ID})
	require.NoError(t, err)

	row, err := env.DB.Suppression.Get(ctx, int64(res.ID))
	require.NoError(t, err)
	assert.NotNil(t, row.LiftedAt, "the lifted row stays with its lift time")
	verdict, err := eligibility.Check(ctx, env.DB.Scoped(fixtures.AcmeID), channels.Email,
		"blocked@example.com", eligibility.SourceBroadcasts)
	require.NoError(t, err)
	assert.True(t, verdict.Eligible, "a lifted destination receives mail again")

	// Lifting it a second time is not-found.
	err = c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: workspaceID, ID: res.ID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)

	// Suppressing the destination again makes a new, active row.
	renewedRes, err := c.SiteSuppressionsCreate(ctx, &siteapi.SiteCreateSuppressionInput{Destination: "blocked@example.com"},
		siteapi.SiteSuppressionsCreateParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	assert.NotEqual(t, res.ID, renewedRes.ID, "a lifted row is not reused")
	assert.False(t, renewedRes.LiftedAt.Set && !renewedRes.LiftedAt.Null, "the new row is active")
}

// The list shows active Suppressions by default and the lifted ones with lifted=true.
func TestSiteSuppressionsListHidesLiftedRows(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)

	activeList, err := c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{WorkspaceId: workspaceID, PageSize: siteapi.NewOptInt32(100)})
	require.NoError(t, err)
	for _, item := range activeList.Items {
		assert.NotEqual(t, siteapi.EntityId(int64(fixtures.SuppressionLiftedID)), item.ID, "a lifted row is hidden by default")
	}

	lifted, err := c.SiteSuppressionsList(ctx, siteapi.SiteSuppressionsListParams{WorkspaceId: workspaceID, Lifted: siteapi.NewOptBool(true)})
	require.NoError(t, err)
	liftedList := lifted
	require.Len(t, liftedList.Items, 1)
	assert.Equal(t, siteapi.EntityId(int64(fixtures.SuppressionLiftedID)), liftedList.Items[0].ID)
	assert.True(t, liftedList.Items[0].LiftedAt.Set)

	err = c.SiteSuppressionsDelete(ctx, siteapi.SiteSuppressionsDeleteParams{WorkspaceId: workspaceID, ID: siteapi.EntityId(int64(fixtures.SuppressionLiftedID))})
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "an already lifted row is not found")
}

// Suppressions are workspace-scoped: an unowned workspaceID is a 404.
func TestSiteSuppressionsRequireOwnedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSuppressionsList(context.Background(), siteapi.SiteSuppressionsListParams{WorkspaceId: "NoSuchWs"})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

// A blank destination is refused on its field, with the message the module's refusal carries.
func TestSiteSuppressionsRefuseABlankDestinationOnItsField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	_, err := c.SiteSuppressionsCreate(t.Context(), &siteapi.SiteCreateSuppressionInput{Destination: "   "},
		siteapi.SiteSuppressionsCreateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID)})

	refused := testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Equal(t, "invalid destination", refused.Detail.Value)
	assert.Equal(t, siteapi.ProblemDetailsErrors{"destination": {"invalid destination"}}, refused.Errors.Value)
}
