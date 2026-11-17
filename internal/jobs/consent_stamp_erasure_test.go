package jobs_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentstamp"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/unsubscriberecording"
)

// An Erasure keeps of the times a person's consent observations occurred only the time of a stop
// that still stands, as it keeps the opt-out (ADR 0154, ADR 0021): the time an address confirmed
// goes with the Confirmation.
func TestErasingAContactKeepsOnlyTheTimeOfTheStopsThatStand(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	address := fixtures.ContactAliceEmail
	require.NoError(t, confirmations.RecordConfirmation(ctx, env.Bus, s, linktarget.Confirmation{
		WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactAliceID, Destination: address,
	}, "", time.Now()))
	_, err := unsubscriberecording.New(env.Bus, env.OutboundModule(), time.Now).Unsubscribe(ctx, s, address, eligibility.SourceBroadcasts)
	require.NoError(t, err)

	eraseContact(t, env, fixtures.ContactAliceID)

	stamps := s.ConsentStamp().Query().Where(
		consentstamp.ChannelEQ(channels.Email), consentstamp.DestinationEQ(address),
	).AllX(ctx)
	sources := make(map[string]bool, len(stamps))
	for _, st := range stamps {
		sources[st.SendingSource] = st.OptOutOccurredAt != nil
		assert.Nil(t, st.ConfirmationOccurredAt, "the time of the erased Confirmation goes")
	}
	assert.Equal(t, map[string]bool{eligibility.SourceBroadcasts: true}, sources,
		"only the time of the opt-out that stands stays, by the address it is kept by")
}
