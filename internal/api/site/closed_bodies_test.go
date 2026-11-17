package site_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// unknownProperty is the field error of a property the contract does not declare.
const unknownProperty = "unknown property: the contract does not declare it"

// A request body is a closed object at every depth (ADR 0148): a property the contract does not
// declare is a 400 naming its path, and the refused create stores nothing.
func TestSiteBroadcastRefusesAPropertyTheContractDoesNotDeclare(t *testing.T) {
	cases := []struct {
		name string
		edit func(body map[string]any)
		path string
	}{
		{"at the top", func(body map[string]any) { body["segment_id"] = "x" }, "segment_id"},
		{"in the content", func(body map[string]any) {
			body["content"].(map[string]any)["subjct"] = "Hello"
		}, "content.subjct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			c := env.SiteActor(t, fixtures.OwnerJohnEmail)
			edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(tc.edit))
			list := siteapi.SiteBroadcastsListParams{WorkspaceId: fixtures.AcmePublicID}
			before, err := c.SiteBroadcastsList(ctx, list)
			require.NoError(t, err)

			_, err = edited.SiteBroadcastsCreate(ctx, &siteapi.CreateBroadcastInput{
				Name:      "Typo",
				Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hello")}),
				SegmentId: siteapi.NilEntityId{Null: true},
			}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})
			after, listErr := c.SiteBroadcastsList(ctx, list)

			require.NoError(t, listErr)
			refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
			reasons := refused.Errors.Or(nil)[tc.path]
			assert.Equal(t, []string{unknownProperty}, reasons)
			assert.Equal(t, before.TotalItems, after.TotalItems, "a refused create stores nothing")
		})
	}
}

// A value of the wrong type gets the same answer, under its own path, with the decoder's reason.
func TestSiteBroadcastRefusesAValueOfTheWrongTypeByItsPath(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		body["content"].(map[string]any)["subject"] = 3
	}))

	_, err := c.SiteBroadcastsCreate(t.Context(), &siteapi.CreateBroadcastInput{
		Name:      "Wrong type",
		Content:   testhelper.SiteEmail(siteapi.EmailBroadcastContentInput{Subject: siteapi.NewOptString("Hello")}),
		SegmentId: siteapi.NilEntityId{Null: true},
	}, siteapi.SiteBroadcastsCreateParams{WorkspaceId: fixtures.AcmePublicID})

	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Contains(t, refused.Errors.Or(nil), "content.subject")
}

// A property nested in a Condition's rule, an item of its rules, is refused by its path through
// the objects (an array adds no segment), and no Segment is stored.
func TestSiteSegmentRefusesAPropertyNestedInAConditionRule(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		rules := body["condition"].(map[string]any)["rules"].([]any)
		rules[0].(map[string]any)["bogus"] = true
	}))
	list := siteapi.SiteSegmentsListParams{WorkspaceId: fixtures.AcmePublicID}
	before, err := c.SiteSegmentsList(ctx, list)
	require.NoError(t, err)

	_, err = edited.SiteSegmentsCreate(ctx, &siteapi.CreateSegmentInput{
		Name:      "Pros",
		Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	after, listErr := c.SiteSegmentsList(ctx, list)

	require.NoError(t, listErr)
	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	reasons := refused.Errors.Or(nil)["condition.rules.0.bogus"]
	assert.Equal(t, []string{unknownProperty}, reasons)
	assert.Equal(t, before.TotalItems, after.TotalItems, "a refused create stores nothing")
}

// A Contact's Custom field values stay open: a key the contract cannot know is taken.
func TestSiteContactCustomFieldsTakeAnyKey(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	created, err := c.SiteContactsCreate(t.Context(), &siteapi.SiteCreateContactInput{
		Email:        siteapi.NewOptNilEmailAddress("open-map@example.com"),
		CustomFields: siteapi.NewOptNilJsonObject(siteapi.JsonObject{"favourite_colour": []byte(`"teal"`)}),
	}, siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	colour := string(created.CustomFields.Value["favourite_colour"])

	assert.JSONEq(t, `"teal"`, colour)
}
