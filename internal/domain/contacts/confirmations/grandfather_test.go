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
// time, each page in a transaction of its own, and returns how many it recorded and the pages it
// took.
func grandfatherTyrell(t *testing.T, env *testhelper.TestEnv, limit int) (recorded, pages int) {
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
		pages++
	}
	return recorded, pages
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

	recorded, _ := grandfatherTyrell(t, env, confirmations.GrandfatherPage)

	for _, mailable := range []string{fixtures.TyrellMatrixCleanEmail, fixtures.TyrellMatrixUnsubBroadcastsEmail, fixtures.TyrellMatrixUnsubAutomationEmail} {
		assert.Equal(t, confirmation.ProvenanceGrandfathered, provenanceOf(t, env, mailable), mailable)
	}
	for _, refused := range []string{fixtures.TyrellMatrixSuppressedEmail, fixtures.TyrellMatrixUnsubEverythingEmail} {
		assert.Empty(t, provenanceOf(t, env, refused), refused)
	}
	assert.Equal(t, confirmation.ProvenanceDoubleOptIn, provenanceOf(t, env, fixtures.TyrellMatrixConfirmedEmail))
	n, err := env.DB.Scoped(fixtures.TyrellID).Confirmation().Query().
		Where(confirmation.ProvenanceEQ(confirmation.ProvenanceGrandfathered)).Count(t.Context())
	require.NoError(t, err)
	assert.Equal(t, n, recorded, "it counts what it recorded")
}

// The list goes in pages, each after the last Contact the one before read, and the pages record
// what one page would.
func TestGrandfatheringInPagesRecordsWhatOnePageWould(t *testing.T) {
	env := testhelper.Setup(t)

	recorded, pages := grandfatherTyrell(t, env, 1)
	again, _ := grandfatherTyrell(t, env, confirmations.GrandfatherPage)

	assert.Greater(t, pages, recorded, "a page per address, and the last finds none")
	assert.Positive(t, recorded)
	assert.Zero(t, again, "a second pass finds nothing left to grandfather")
}
