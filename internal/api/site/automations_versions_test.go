package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The resource reads the live version and the draft side by side (ADR 0065). An Update writes the
// draft only; Publish makes it live, and with no draft it resumes the live version as it was; a
// Revert with no draft above a live version is a 422.
func TestSiteAutomationsReadLiveAndDraftAndUpdateWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	workspaceID := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := idStr(fixtures.AutomationHoldDemoID)
	get := func() *siteapi.SiteAutomationResource {
		t.Helper()
		res, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: workspaceID, ID: id})
		require.NoError(t, err)
		got, ok := res.(*siteapi.SiteAutomationResource)
		require.Truef(t, ok, "got %T", res)
		return got
	}

	before := get()
	assert.EqualValues(t, 1, before.Live.Value.Number)
	assert.True(t, before.Draft.Null, "a published Automation with no edit has no draft")

	_, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		Name:         siteapi.NewOptString("Hold demo, edited"),
		TriggerEvent: siteapi.NewOptString("hold_demo_v2"),
		Steps:        []siteapi.SiteAutomationStep{{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(60)}},
	}, siteapi.SiteAutomationsUpdateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	edited := get()
	assert.Equal(t, "Hold demo, edited", edited.Name, "the name is not versioned")
	assert.Equal(t, before.Live.Value, edited.Live.Value, "the live version is untouched")
	assert.EqualValues(t, 2, edited.Draft.Value.Number)
	assert.Equal(t, "hold_demo_v2", edited.Draft.Value.TriggerEvent)
	assert.Equal(t, []siteapi.SiteAutomationStep{{Type: siteapi.SiteAutomationStepTypeWait, Seconds: siteapi.NewOptInt32(60)}}, edited.Draft.Value.Steps)

	_, err = c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	published := get()
	assert.Equal(t, edited.Draft.Value, published.Live.Value, "the draft became live")
	assert.True(t, published.Draft.Null)

	_, err = c.SiteAutomationsDeactivate(ctx, siteapi.SiteAutomationsDeactivateParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	_, err = c.SiteAutomationsPublish(ctx, siteapi.SiteAutomationsPublishParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	resumed := get()
	assert.Equal(t, siteapi.SiteAutomationStatusActive, resumed.Status)
	assert.Equal(t, published.Live.Value, resumed.Live.Value, "Publish without a draft resumes the live version")

	refused, err := c.SiteAutomationsRevert(ctx, siteapi.SiteAutomationsRevertParams{WorkspaceId: workspaceID, ID: id})
	require.NoError(t, err)
	unprocessable, ok := refused.(*siteapi.SiteAutomationsRevertUnprocessableEntity)
	require.Truef(t, ok, "got %T", refused)
	assert.Equal(t, "no unpublished changes to revert", unprocessable.Detail.Or(""))
}
