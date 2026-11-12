package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Two Saves of a Segment made on one revision: the second is a 409 revision_changed, and the
// Condition is the first Save's (ADR 0119).
func TestSiteSegmentSaveOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentProPlanID)}
	read, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams(params))
	require.NoError(t, err)
	firstCondition := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"has","value":"first"}]}`
	secondCondition := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"has","value":"second"}]}`

	first, err := c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{
		Revision: read.Revision, Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, firstCondition)),
	}, params)
	require.NoError(t, err)
	_, err = c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{
		Revision: read.Revision, Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, secondCondition)),
	}, params)

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	assert.Equal(t, read.Revision+1, first.Revision)
	stored := env.DB.Segment.GetX(ctx, fixtures.SegmentProPlanID)
	require.NotNil(t, stored.Condition)
	assert.JSONEq(t, firstCondition, *stored.Condition)
	assert.EqualValues(t, first.Revision, stored.Revision)
}

// An update that names no revision is refused by the contract and writes nothing.
func TestSiteSegmentUpdateWithoutARevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.DropField("revision"))
	ctx := context.Background()
	before := env.DB.Segment.GetX(ctx, fixtures.SegmentProPlanID)

	_, err := c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Revision: siteapi.Revision(before.Revision), Name: siteapi.NewOptName("Blind")},
		siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentProPlanID)})

	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	after := env.DB.Segment.GetX(ctx, fixtures.SegmentProPlanID)
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.Revision, after.Revision)
}
