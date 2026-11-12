package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalTagsRemoveForAnUnknownOrForeignContactIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite)

	for name, id := range map[string]externalapi.EntityId{
		"unknown": 999999,
		"foreign": entityIDString(fixtures.ContactGlobexID),
	} {
		t.Run(name, func(t *testing.T) {
			err := c.TagsRemove(ctx, externalapi.TagsRemoveParams{ContactId: id, Name: "vip"})
			testhelper.ExternalRefusal(t, err, http.StatusNotFound)
		})
	}
}

func TestExternalContactsCreatePersistsCustomFields(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, apitokens.ScopeContactsRead, apitokens.ScopeContactsWrite)

	created, err := c.ContactsCreate(ctx, &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress("fields@example.com"),
		CustomFields: externalapi.NewOptNilJsonObject(externalapi.JsonObject{
			"plan": jx.Raw(`"pro"`),
		}),
	})
	require.NoError(t, err)

	stored, err := env.DB.Contact.Get(ctx, int64(created.ID))
	require.NoError(t, err)
	assert.Equal(t, "pro", stored.CustomFields["plan"])
}
