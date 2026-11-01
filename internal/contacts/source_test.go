package contacts_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestACreatedContactKeepsTheDoorItCameThrough(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	acme := env.DB.Scoped(fixtures.AcmeID)

	byHand, err := m.Create(context.Background(), acme, contact.SourceManual, contacts.CreateInput{Email: new("door-hand@example.com")})
	require.NoError(t, err)
	viaAPI, err := m.Upsert(context.Background(), acme, contact.SourceAPI, contacts.UpsertInput{Email: new("door-api@example.com")})
	require.NoError(t, err)
	batch := m.UpsertBatch(context.Background(), acme, contact.SourceAPI, []contacts.UpsertInput{{Email: new("door-batch@example.com")}})
	require.NoError(t, batch[0].Err)

	assert.Equal(t, contact.SourceManual, lo.FromPtr(byHand.Source))
	assert.Equal(t, contact.SourceAPI, lo.FromPtr(viaAPI.Contact.Source))
	assert.Equal(t, contact.SourceAPI, lo.FromPtr(batch[0].Result.Contact.Source))
}

func TestALaterUpsertLeavesTheSourceUnchanged(t *testing.T) {
	env := testhelper.Setup(t)
	m := contacts.New(env.Bus)
	acme := env.DB.Scoped(fixtures.AcmeID)

	created, err := m.Create(context.Background(), acme, contact.SourceManual, contacts.CreateInput{Email: new("same-person@example.com")})
	require.NoError(t, err)
	again, err := m.Upsert(context.Background(), acme, contact.SourceTracker, contacts.UpsertInput{Email: new("same-person@example.com"), FirstName: patch.Set("Sam")})
	require.NoError(t, err)

	assert.False(t, again.Created)
	assert.Equal(t, created.ID, again.Contact.ID)
	assert.Equal(t, contact.SourceManual, lo.FromPtr(again.Contact.Source))
}

func TestAContactThatPredatesTheFieldHasNoSource(t *testing.T) {
	env := testhelper.Setup(t)
	alice, err := env.DB.Scoped(fixtures.AcmeID).Contact().Get(context.Background(), fixtures.ContactAliceID)
	require.NoError(t, err)

	assert.Nil(t, alice.Source)
}
