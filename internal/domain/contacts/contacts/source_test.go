package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/contactintake"
)

func TestACreatedContactKeepsTheDoorItCameThrough(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	acme := env.DB.Scoped(fixtures.AcmeID)

	byHand, err := m.Create(context.Background(), acme, contact.SourceManual, contacts.CreateInput{Email: new("door-hand@example.com")})
	require.NoError(t, err)
	viaAPI, err := m.Upsert(context.Background(), acme, contact.SourceAPI, contacts.UpsertInput{Email: new("door-api@example.com")})
	require.NoError(t, err)
	batch := contactintake.New(env.Bus, m).UpsertBatch(context.Background(), acme, contact.SourceAPI, []contactintake.Input{{Email: new("door-batch@example.com")}})
	require.NoError(t, batch.Outcomes[0].Err)

	assert.Equal(t, contact.SourceManual, lo.FromPtr(byHand.Source))
	assert.Equal(t, contact.SourceAPI, lo.FromPtr(viaAPI.Contact.Source))
	assert.Equal(t, contact.SourceAPI, lo.FromPtr(batch.Outcomes[0].Value.Contact.Source))
}

func TestALaterUpsertLeavesTheSourceUnchanged(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)

	again, err := m.Upsert(context.Background(), env.DB.Scoped(fixtures.StarkID), contact.SourceTracker, contacts.UpsertInput{Email: new(fixtures.ContactStarkManualEmail), FirstName: patch.Set("Sam")})
	require.NoError(t, err)

	assert.False(t, again.Created)
	assert.EqualValues(t, fixtures.ContactStarkManualID, again.Contact.ID)
	assert.Equal(t, contact.SourceManual, lo.FromPtr(again.Contact.Source))
}

func TestAContactThatPredatesTheFieldHasNoSource(t *testing.T) {
	env := testhelper.Setup(t)
	alice, err := env.DB.Scoped(fixtures.AcmeID).Contact().Get(context.Background(), fixtures.ContactAliceID)
	require.NoError(t, err)

	assert.Nil(t, alice.Source)
}
