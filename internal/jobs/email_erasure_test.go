package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An email delivery notice is stored under the provider's event id and the address it names
// (callbackinterpretation), so the Erasure that keeps the notice as an anonymous count clears its
// source id as well: no Event outlives the Contact with the address in it (ADR 0021).
func TestErasingAContactLeavesItsAddressInNoSourceIdOfItsDeliveryNotices(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	address := fixtures.ContactBobEmail
	require.NoError(t, env.Bus.Publish(ctx, &events.EmailDelivery{
		Action: systemevent.EmailBounced, WorkspaceID: fixtures.AcmeID, ContactID: fixtures.ContactBobID,
		Email: address, Provider: "ses", BounceKind: systemevent.BounceKindHard, DedupID: "sns-erasure/" + address,
	}))
	bounce := env.DB.Event.Query().Where(event.ContactID(fixtures.ContactBobID), event.Action(systemevent.EmailBounced)).OnlyIDX(ctx)

	eraseContact(t, env, fixtures.ContactBobID)

	kept := env.DB.Event.GetX(ctx, bounce)
	assert.Nil(t, kept.ContactID, "the bounce outlives the Contact as an anonymous count")
	assert.Nil(t, kept.SourceID, "its source id, which names the address, is gone")
	assert.Zero(t, env.DB.Event.Query().Where(event.SourceIDContains(address)).CountX(ctx), "no Event's source id names the address")
}
