package contacts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestACreatedContactKeepsTheDoorItCameThrough(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*contacts.Module](env)
	acme := env.DB.Scoped(fixtures.AcmeID)

	byHand, err := m.Create(t.Context(), acme, contact.SourceManual, contacts.CreateInput{Email: new("door-hand@example.com")})
	require.NoError(t, err)
	viaAPI, err := m.Upsert(t.Context(), acme, contact.SourceAPI, contacts.UpsertInput{Email: new("door-api@example.com")})
	require.NoError(t, err)
	batch, err := m.UpsertBatch(t.Context(), acme, contact.SourceAPI, []contacts.IntakeInput{{Email: new("door-batch@example.com")}})
	require.NoError(t, err)
	require.NoError(t, batch.Items[0].Err)

	assert.Equal(t, contact.SourceManual, byHand.Source)
	assert.Equal(t, contact.SourceAPI, viaAPI.Contact.Source)
	assert.Equal(t, contact.SourceAPI, batch.Items[0].Value.Contact.Source)
}

func TestALaterUpsertLeavesTheSourceUnchanged(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*contacts.Module](env)

	again, err := m.Upsert(t.Context(), env.DB.Scoped(fixtures.StarkID), contact.SourceTracker, contacts.UpsertInput{Email: new(fixtures.ContactStarkManualEmail), FirstName: patch.Set("Sam")})
	require.NoError(t, err)

	assert.False(t, again.Created)
	assert.EqualValues(t, fixtures.ContactStarkManualID, again.Contact.ID)
	assert.Equal(t, contact.SourceManual, again.Contact.Source)
}
