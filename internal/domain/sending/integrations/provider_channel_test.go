package integrations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses an Integration whose channel is not its provider's, whatever code writes
// it: such a row would be picked as that Channel's default and routed to the wrong sender
// (integrations_provider_channel_check).
func TestTheDatabaseRefusesAProviderOnAnotherChannel(t *testing.T) {
	env := testhelper.Setup(t)
	// test writes: the refused insert is the operation under test.
	err := env.DB.Integration.Create().SetWorkspaceID(fixtures.AcmeID).SetName("Mismatched").
		SetChannel(channels.Email).SetProvider(providers.Twilio).SetConfigEncrypted("{}").
		SetIngestKey("omik_test_mismatched").Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
}
