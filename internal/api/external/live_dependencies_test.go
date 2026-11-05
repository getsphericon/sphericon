package external_test

import (
	"context"
	"net/http"
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

	err := c.TagsDelete(ctx, externalapi.TagsDeleteParams{ID: externalapi.EntityId(fixtures.TagVipID)})
	refused := testhelper.ExternalRefusal(t, err, http.StatusConflict)
	names := map[externalapi.UsedByKind][]string{}
	for _, u := range refused.UsedBy {
		names[u.Kind] = append(names[u.Kind], u.Name)
	}
	assert.Equal(t, map[externalapi.UsedByKind][]string{
		externalapi.UsedByKindAutomation: {fixtures.AutomationTagOnEngagementName},
		externalapi.UsedByKindRule:       {fixtures.RuleVipOnlyName, fixtures.RuleMultiTriggerName},
	}, names)

	err = c.IntegrationsDelete(ctx, externalapi.IntegrationsDeleteParams{ID: externalapi.EntityId(fixtures.IntegrationAcmeDefaultID)})
	assert.NotEmpty(t, testhelper.ExternalRefusal(t, err, http.StatusConflict).UsedBy)

	err = c.SendingDomainsDelete(ctx, externalapi.SendingDomainsDeleteParams{ID: externalapi.EntityId(fixtures.SendingDomainCodebasicsID)})
	assert.NotEmpty(t, testhelper.ExternalRefusal(t, err, http.StatusConflict).UsedBy)

	err = c.TagsDelete(ctx, externalapi.TagsDeleteParams{ID: externalapi.EntityId(fixtures.TagUnusedID)})
	require.NoError(t, err)
}
