package collect_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Erasure keeps no tombstone (ADR 0021): a person who reappears through Identify is a
// fresh Contact, and the Unsubscribe and Suppression that survived the Erasure still
// refuse mail to their address.
func TestCollectIdentifyAfterErasureCreatesAFreshContactStillRefusedByTheSurvivingOptOuts(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	erased, err := env.ExternalWithToken(t, env.ScopedBearerFor(t, fixtures.InitechID, "contacts:erase")).
		ContactsDelete(ctx, externalapi.ContactsDeleteParams{ID: externalapi.EntityId(fixtures.ContactErasableID)})
	require.NoError(t, err)
	require.IsType(t, &externalapi.ContactsDeleteNoContent{}, erased)

	res, err := env.CollectWithKey(t, fixtures.InitechCollectKey).CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "returning-visitor",
		Email:     collectapi.NewOptNilEmailAddress(fixtures.ContactErasableEmail),
	})
	require.NoError(t, err)
	require.IsType(t, &collectapi.CollectOkResponse{}, res)

	fresh, err := env.DB.Contact.Query().
		Where(contact.WorkspaceID(fixtures.InitechID), contact.Email(fixtures.ContactErasableEmail)).Only(ctx)
	require.NoError(t, err, "Identify created a new Contact for the returning address")
	assert.NotEqual(t, int64(fixtures.ContactErasableID), fresh.ID, "it is not the erased row")

	// Suppression is checked before Unsubscribe, so the opt-outs are told apart by
	// removing the Suppression for the second check.
	s := env.DB.Scoped(fixtures.InitechID)
	transactional, err := eligibility.Check(ctx, s, "email", fixtures.ContactErasableEmail, "")
	require.NoError(t, err)
	assert.False(t, transactional.Eligible, "the surviving Suppression refuses even a transactional send")
	assert.Equal(t, eligibility.ReasonSuppressed, transactional.Reason)

	require.NoError(t, env.DB.Suppression.DeleteOneID(fixtures.SuppressionErasableID).Exec(ctx))
	broadcasts, err := eligibility.Check(ctx, s, "email", fixtures.ContactErasableEmail, "broadcasts")
	require.NoError(t, err)
	assert.False(t, broadcasts.Eligible, "the surviving Unsubscribe refuses the new Contact")
	assert.Equal(t, eligibility.ReasonUnsubscribedSource, broadcasts.Reason)
}
