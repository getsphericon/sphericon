package site_test

import (
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Params the Preset's schema refuses are refused on the field of each param (params.<name>), so
// the form generated from the schema shows each refusal on its own field.
func TestSiteIntegrationsRefuseParamsOnTheirOwnField(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	create := func(params siteapi.JsonObject) siteapi.ProblemDetailsErrors {
		t.Helper()
		res, err := c.SiteIntegrationsCreate(t.Context(), &siteapi.CreateIntegrationInput{
			Name:   "SES",
			Config: sesInput("eu-west-1", "AKIA1234", "secret"),
			Params: siteapi.NewOptJsonObject(params),
		}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		require.NoError(t, err)
		refused, ok := res.(*siteapi.SiteIntegrationsCreateUnprocessableEntity)
		require.Truef(t, ok, "got %T", res)
		return refused.Errors.Value
	}

	pattern := create(siteapi.JsonObject{"topic_arn": jx.Raw(`"not-an-arn"`)})
	missing := create(siteapi.JsonObject{})
	extra := create(siteapi.JsonObject{
		"topic_arn": jx.Raw(`"arn:aws:sns:eu-west-1:123456789012:acme-ses-events"`),
		"region":    jx.Raw(`"eu-west-1"`),
	})

	assert.Contains(t, pattern, "params.topic_arn", "a value off its pattern is refused on its field")
	assert.Contains(t, missing, "params.topic_arn", "a missing param is refused on its field")
	assert.Contains(t, extra, "params.region", "a param the schema does not have is refused on its field")
}
