package collect_test

import (
	"context"
	"testing"

	"github.com/go-faster/jx"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/customfield"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// ADR 0072: a trait named after a core attribute writes the attribute, and the browser's context
// fills only what the Contact has none of.
func TestIdentifyWritesCoreAttributesFromTraitsAndFillsFromTheBrowser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.CollectAcme(t)
	ctx := context.Background()

	_, err := c.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "v-core",
		Email:     collectapi.NewOptNilEmailAddress("core.traits@example.com"),
		Traits: collectapi.NewOptNilJsonObject(collectapi.JsonObject{
			"firstName": jx.Raw(`"Ada"`),
			"country":   jx.Raw(`"gb"`),
			"timeZone":  jx.Raw(`"Mars/Olympus"`),
		}),
		Context: collectapi.NewOptNilCollectIdentifyContext(collectapi.CollectIdentifyContext{
			TimeZone: collectapi.NewOptNilString("Europe/London"),
			Locale:   collectapi.NewOptNilString("en-GB"),
		}),
	})
	require.NoError(t, err)

	ct, err := env.DB.Contact.Query().Where(contact.Email("core.traits@example.com")).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Ada", lo.FromPtr(ct.FirstName))
	assert.Equal(t, "GB", lo.FromPtr(ct.Country))
	assert.Equal(t, "Europe/London", lo.FromPtr(ct.TimeZone), "an invalid trait is dropped, so the browser's zone fills")
	assert.Equal(t, "en-GB", lo.FromPtr(ct.Locale))
	assert.Empty(t, ct.CustomFields, "no trait became a Custom field")

	exists, err := env.DB.CustomField.Query().
		Where(customfield.WorkspaceID(fixtures.AcmeID), customfield.KeyIn("firstName", "country", "timeZone")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)

	_, err = c.CollectIdentifyCreate(ctx, &collectapi.CollectIdentifyInput{
		VisitorId: "v-core",
		Email:     collectapi.NewOptNilEmailAddress("core.traits@example.com"),
		Traits:    collectapi.NewOptNilJsonObject(collectapi.JsonObject{"firstName": jx.Raw(`"Augusta"`), "country": jx.Raw(`null`)}),
		Context: collectapi.NewOptNilCollectIdentifyContext(collectapi.CollectIdentifyContext{
			TimeZone: collectapi.NewOptNilString("Asia/Tokyo"),
		}),
	})
	require.NoError(t, err)

	again, err := env.DB.Contact.Get(ctx, ct.ID)
	require.NoError(t, err)
	assert.Equal(t, "Augusta", lo.FromPtr(again.FirstName), "a trait overwrites")
	assert.Nil(t, again.Country, "a null trait clears")
	assert.Equal(t, "Europe/London", lo.FromPtr(again.TimeZone), "the browser never replaces a stored zone")
}
