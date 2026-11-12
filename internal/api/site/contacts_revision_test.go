package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/tag"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Contact form loaded before an Automation's Tag step added a Tag: its Save is a 409
// revision_changed, and the Tag stays (ADR 0119). The step applies the Tag with ApplyTagIn, which
// ApplyTag runs in a transaction of its own.
func TestSiteContactSaveAfterATagWasAppliedIsRefusedAndTheTagStays(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteContactsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.ContactAliceID)}
	form, err := c.SiteContactsGet(ctx, params)
	require.NoError(t, err)
	before := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)

	_, err = contacts.New(env.Bus).ApplyTag(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.ContactAliceID, "from-automation")
	require.NoError(t, err)

	// The form sends the Tags it shows, which lack the one the step applied.
	_, err = c.SiteContactsUpdate(ctx, &siteapi.SiteUpdateContactInput{
		Revision: form.Revision, FirstName: siteapi.NewOptNilString("Renamed"), Tags: form.Tags,
	}, siteapi.SiteContactsUpdateParams(params))

	refused := testhelper.SiteRefusal(t, err, http.StatusConflict)
	assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
	stored := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID)
	assert.Equal(t, before.FirstName, stored.FirstName, "nothing of the Save is written")
	names := env.DB.Contact.QueryTags(stored).Select(tag.FieldName).StringsX(ctx)
	assert.Contains(t, names, "from-automation")
	assert.Equal(t, before.Revision+1, stored.Revision, "the Tag moved the revision once")
}
