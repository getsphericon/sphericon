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
	names := map[externalapi.UsedByKind][]string{}
	for _, u := range refused.UsedBy {
		names[u.Kind] = append(names[u.Kind], u.Name)
	}
	assert.Equal(t, map[externalapi.UsedByKind][]string{
		externalapi.UsedByKindAutomation: {fixtures.AutomationTagOnEngagementName},
		externalapi.UsedByKindRule:       {fixtures.RuleVipOnlyName, fixtures.RuleMultiTriggerName},
	}, names)

	integration, err := c.IntegrationsArchive(ctx, externalapi.IntegrationsArchiveParams{ID: externalapi.EntityId(1)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.InUseProblemDetails{}, integration)

	domain, err := c.SendingDomainsArchive(ctx, externalapi.SendingDomainsArchiveParams{ID: externalapi.EntityId(3)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.InUseProblemDetails{}, domain)

	free, err := c.TagsDelete(ctx, externalapi.TagsDeleteParams{ID: externalapi.EntityId(3)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.TagsDeleteNoContent{}, free)
}
