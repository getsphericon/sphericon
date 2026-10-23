package external_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestContactsCreatedThroughTheExternalAPICarryTheAPISource(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	ctx := context.Background()

	_, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("source-single@example.com")})
	require.NoError(t, err)
	_, err = c.ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{
		Contacts: []externalapi.UpsertContactInput{{Email: externalapi.NewOptNilEmailAddress("source-batch@example.com")}},
	})
	require.NoError(t, err)

	single, err := env.DB.Contact.Query().Where(contact.Email("source-single@example.com")).Only(ctx)
	require.NoError(t, err)
	batch, err := env.DB.Contact.Query().Where(contact.Email("source-batch@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, contact.SourceAPI, lo.FromPtr(single.Source))
	assert.Equal(t, contact.SourceAPI, lo.FromPtr(batch.Source))
}
