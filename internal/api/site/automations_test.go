package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteAutomationsCRUDAndActivation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	slug := fixtures.AcmeSlug

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.SiteCreateAutomationInput{
		Name:         "Welcome series",
		TriggerEvent: "contact.created",
		Steps: []siteapi.SiteAutomationStep{
			{Type: siteapi.SiteAutomationStepTypeEmail, Subject: siteapi.NewOptString("Hi"), Body: siteapi.NewOptString("<p></p>")},
			{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(3600)},
		},
	}, siteapi.SiteAutomationsCreateParams{Slug: slug})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, "Welcome series", res.Name)
	assert.Equal(t, siteapi.SiteAutomationStatusDraft, res.Status)
	assert.Equal(t, "contact.created", res.TriggerEvent)

	// Steps round-trip through the typed contract (stored as the executor's JSON
	// string, decoded back into the typed DTO).
	require.Len(t, res.Steps, 2)
	assert.Equal(t, siteapi.SiteAutomationStepTypeEmail, res.Steps[0].Type)
	assert.Equal(t, "Hi", res.Steps[0].Subject.Or(""))
	assert.Equal(t, siteapi.SiteAutomationStepTypeWait, res.Steps[1].Type)
	assert.Equal(t, int32(3600), res.Steps[1].Seconds.Or(0))

	// Activate / deactivate flip the status.
	act, err := c.SiteAutomationsActivate(ctx, siteapi.SiteAutomationsActivateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	actRes, ok := act.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", act)
	assert.Equal(t, siteapi.SiteAutomationStatusActive, actRes.Status)

	deact, err := c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	deactRes, ok := deact.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", deact)
	assert.Equal(t, siteapi.SiteAutomationStatusDraft, deactRes.Status)

	// Fetch the created automation back by id (selection by key).
	got, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	gotRes, ok := got.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, res.ID, gotRes.ID)

	// Update renames.
	upd, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		Name: siteapi.NewOptString("Onboarding"),
	}, siteapi.SiteAutomationsUpdateParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	updRes, ok := upd.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", upd)
	assert.Equal(t, "Onboarding", updRes.Name)

	// Delete, then a fetch by id resolves to 404.
	del, err := c.SiteAutomationsDelete(ctx, siteapi.SiteAutomationsDeleteParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsDeleteNoContent{}, del)

	gone, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{Slug: slug, ID: res.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsGetNotFound{}, gone)
}
