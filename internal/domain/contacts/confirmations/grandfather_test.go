package confirmations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/confirmation"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// grandfatherTyrell grandfathers Tyrell's eligibility matrix page by page, limit addresses at a
// time, each page in a transaction of its own, and returns how many it recorded.
func grandfatherTyrell(t *testing.T, env *testhelper.TestEnv, limit int) (recorded int) {
	t.Helper()
	confs := testhelper.Module[*confirmations.Module](env)
	bus := testhelper.Module[*events.Bus](env)
	page := confirmations.Grandfathered{More: true}
	for page.More {
		err := bus.WithinScopedTx(t.Context(), env.DB.Scoped(fixtures.TyrellID), func(ts *ent.Scoped, _ events.Publisher) error {
			var err error
			page, err = confs.Grandfather(t.Context(), ts, page.Last, limit)
			return err
		})
		require.NoError(t, err)
		recorded += page.Recorded
	}
	return recorded
}

// provenanceOf is the provenance of the email Confirmation of address in Tyrell, "" when it has none.
func provenanceOf(t *testing.T, env *testhelper.TestEnv, address string) confirmation.Provenance {
	t.Helper()
	c, err := env.DB.Scoped(fixtures.TyrellID).Confirmation().Query().
		Where(confirmation.ChannelEQ(channels.Email), confirmation.DestinationEQ(address)).Only(t.Context())
	if ent.IsNotFound(err) {
		return ""
	}
	require.NoError(t, err)
	return c.Provenance
}

// Turning confirmed opt-in on asserts consent for the addresses mailable then (ADR 0013): a
// per-source opt-out leaves an address mailable from everything else, so it is grandfathered; a
// Suppression or an opt-out of everything is not, and a stronger Confirmation stands.
func TestGrandfatheringCoversTheMailableAddressesOnly(t *testing.T) {
	env := testhelper.Setup(t)

	recorded := grandfatherTyrell(t, env, confirmations.GrandfatherPage)

	got := map[string]confirmation.Provenance{}
	for _, address := range []string{
		fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubBroadcastsEmail, fixtures.TyrellMatrixUnsubAutomationEmail,
		fixtures.TyrellMatrixSuppressedEmail, fixtures.TyrellMatrixUnsubEverythingEmail, fixtures.TyrellMatrixConfirmedEmail,
	} {
		got[address] = provenanceOf(t, env, address)
	}
	n, err := env.DB.Scoped(fixtures.TyrellID).Confirmation().Query().
		Where(confirmation.ProvenanceEQ(confirmation.ProvenanceGrandfathered)).Count(t.Context())
	require.NoError(t, err)

	assert.Equal(t, map[string]confirmation.Provenance{
		fixtures.TyrellMatrixCleanEmail:           confirmation.ProvenanceGrandfathered,
		fixtures.TyrellMatrixUnsubBroadcastsEmail: confirmation.ProvenanceGrandfathered,
		fixtures.TyrellMatrixUnsubAutomationEmail: confirmation.ProvenanceGrandfathered,
		fixtures.TyrellMatrixSuppressedEmail:      "",
		fixtures.TyrellMatrixUnsubEverythingEmail: "",
		fixtures.TyrellMatrixConfirmedEmail:       confirmation.ProvenanceDoubleOptIn,
	}, got)
	assert.Equal(t, n, recorded, "it counts what it recorded")
}

// The list goes in pages, each after the last Contact the one before read, and the pages record
// what one page would.
func TestGrandfatheringInPagesRecordsWhatOnePageWould(t *testing.T) {
	env := testhelper.Setup(t)

	inPages := grandfatherTyrell(t, env, 1)
	again := grandfatherTyrell(t, env, confirmations.GrandfatherPage)

	assert.Positive(t, inPages)
	assert.Zero(t, again, "a second pass finds nothing left to grandfather")
}
