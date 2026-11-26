package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
)

// AcmeConfirmationFrom is the From of the confirmation email SetUpAcmeConfirmationEmail sets up: an
// address on Acme's verified Sending domain.
const AcmeConfirmationFrom = "hello@" + fixtures.SendingDomainVerifiedDomain

// SetUpAcmeConfirmationEmail sets up Acme's confirmation email (ADR 0013) as its Owner does on the
// Workspace's settings, its texts left to their defaults.
func (env *TestEnv) SetUpAcmeConfirmationEmail(t *testing.T) {
	t.Helper()
	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesUpdateConfirmationEmail(t.Context(),
		&siteapi.SiteUpdateConfirmationEmailInput{
			Revision:  siteapi.Revision(env.WorkspaceRevision(t.Context(), t, string(fixtures.AcmePublicID))),
			FromEmail: AcmeConfirmationFrom,
		},
		siteapi.SiteWorkspacesUpdateConfirmationEmailParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
}
