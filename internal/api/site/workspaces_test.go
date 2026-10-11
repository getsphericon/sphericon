package site_test

import (
	"context"
	"testing"
	"time"

	siteapi "github.com/mokevnin/sphericon/gen/site"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/suspension"
	"github.com/mokevnin/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteWorkspacesUpdateRenames(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteWorkspacesUpdate(ctx,
		&siteapi.SiteUpdateWorkspaceInput{Name: "Acme Inc"},
		siteapi.SiteWorkspacesUpdateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	updated, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Acme Inc", updated.Name)
	assert.Equal(t, fixtures.AcmeSlug, updated.Slug, "slug is immutable")

	// The change is reflected in the list.
	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Acme Inc", list[0].Name)
}

func TestSiteWorkspacesUpdateNotOwned(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "Hijacked"},
		siteapi.SiteWorkspacesUpdateParams{Slug: "does-not-exist"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateNotFound{}, res)
}

func TestSiteWorkspacesUpdateRejectsBlankName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "   "},
		siteapi.SiteWorkspacesUpdateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateUnprocessableEntity{}, res)
}

// The owner sees a suspension (ADR 0007): the workspace resource carries when it was
// suspended and why, so the dashboard can say so while login and reads keep working.
func TestSiteWorkspacesListExposesSuspension(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	_, suspended := list[0].SuspendedAt.Get()
	assert.False(t, suspended, "a workspace that can send carries no suspension")

	_, err = suspension.SuspendWorkspace(ctx, env.Bus, 1, suspension.System, "complaint rate above 0.3%")
	require.NoError(t, err)

	list, err = c.SiteWorkspacesList(ctx)
	require.NoError(t, err, "reads keep working while suspended")
	at, suspended := list[0].SuspendedAt.Get()
	require.True(t, suspended)
	assert.False(t, time.Time(at).IsZero())
	reason, _ := list[0].SuspensionReason.Get()
	assert.Equal(t, "complaint rate above 0.3%", reason)
}
