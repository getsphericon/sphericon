package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The external API refuses the same removals as /site with a 409 that names the users (ADR 0049).
func TestExternalDeletesRefusedWhileLiveWorkUsesTheRow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:write", "integrations:write", "sending_domains:write")

	tag, err := c.TagsDelete(ctx, externalapi.TagsDeleteParams{ID: externalapi.EntityId(1)})
	require.NoError(t, err)
	refused, ok := tag.(*externalapi.InUseProblemDetails)
	require.Truef(t, ok, "got %T", tag)
	require.Len(t, refused.UsedBy, 1)
	assert.Equal(t, externalapi.UsedByKindAutomation, refused.UsedBy[0].Kind)
	assert.Equal(t, fixtures.AutomationTagOnEngagementName, refused.UsedBy[0].Name)

	integration, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: externalapi.EntityId(1)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.InUseProblemDetails{}, integration)

	domain, err := c.SendingDomainsDelete(ctx, externalapi.SendingDomainsDeleteParams{ID: externalapi.EntityId(3)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.InUseProblemDetails{}, domain)

	free, err := c.TagsDelete(ctx, externalapi.TagsDeleteParams{ID: externalapi.EntityId(3)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsDeleteNoContent{}, free)
}
