package messaging_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// New work may name an Integration of the Workspace that is kept; an archived one leaves new
// work (ADR 0049), and one of another Workspace or none at all is not there.
func TestIntegrationKeptIsTrueOnlyForAKeptIntegrationOfTheWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)

	for name, tc := range map[string]struct {
		id   int64
		want bool
	}{
		"kept":                  {fixtures.IntegrationAcmeMailgunID, true},
		"archived":              {fixtures.IntegrationAcmeArchivedID, false},
		"another workspace's":   {fixtures.IntegrationGlobexID, false},
		"no such Integration":   {999999, false},
		"kept but not default":  {fixtures.IntegrationAcmeSesID, true},
		"the Workspace default": {fixtures.IntegrationAcmeDefaultID, true},
	} {
		got, err := messaging.IntegrationKept(t.Context(), acme, tc.id)
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}
}
