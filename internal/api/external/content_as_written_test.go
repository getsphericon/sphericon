package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The external API stores a body as it was written, like the site API (ADR 0118).
func TestExternalStoresBodiesAsWritten(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeTemplatesWrite, apitokens.ScopeBroadcastsWrite)
	ctx := context.Background()
	body := `<p onclick="track()">Hi {{ first_name }}</p><form action="https://example.com/survey"></form>`

	tpl, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "As written", Body: externalapi.NewOptString(body),
	})
	require.NoError(t, err)
	assert.Equal(t, body, tpl.Body)

	bc, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:    "As written",
		Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Body: externalapi.NewOptString(body)}),
	})
	require.NoError(t, err)
	assert.Equal(t, body, testhelper.ExternalEmailOf(bc).Body)
}
