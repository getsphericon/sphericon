package external_test

import (
	"context"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalTagsRemoveForAnUnknownOrForeignContactIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")

	for name, id := range map[string]externalapi.EntityId{
		"unknown": 999999,
		"foreign": entityIDString(fixtures.ContactGlobexID),
	} {
		t.Run(name, func(t *testing.T) {
			res, err := c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: id, Name: "vip"})
			require.NoError(t, err)
			assert.IsType(t, &externalapi.TagsRemoveNotFound{}, res)
		})
	}
}

func TestExternalContactsCreatePersistsCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "contacts:read", "contacts:write")

	res, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress("fields@example.com"),
		CustomFields: externalapi.NewOptNilCreateContactInputCustomFields(externalapi.CreateContactInputCustomFields{
			"plan": jx.Raw(`"pro"`),
		}),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.ContactResource)
	require.Truef(t, ok, "got %T", res)

	stored, err := env.DB.Contact.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "pro", stored.CustomFields["plan"])
}
