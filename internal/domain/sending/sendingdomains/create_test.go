package sendingdomains_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Acme keeps launch.acme.com: adding it again breaks the index that keeps a domain once per
// Workspace.
func TestCreateRefusesADomainTheWorkspaceKeeps(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := sendingdomains.New(env.DB, env.Bus, env.Cipher, nil, testhelper.Module[*integrations.Module](env), time.Now).
		Create(t.Context(), env.DB.Scoped(fixtures.AcmeID), sendingdomains.CreateInput{Domain: fixtures.SendingDomainLaunchDomain})

	require.ErrorIs(t, err, sendingdomains.ErrAlreadyExists)
}
