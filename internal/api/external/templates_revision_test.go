package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// /api refuses a stale revision as /site does: a 409 revision_changed, nothing written (ADR 0119).
func TestExternalTemplateUpdateOnAStaleRevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:read", "templates:write")
	ctx := context.Background()
	id := entityIDString(fixtures.TemplateWelcomeID)
	read, err := c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: id})
	require.NoError(t, err)

	first, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: read.Revision, Subject: externalapi.NewOptString("First")}, externalapi.TemplatesUpdateParams{ID: id})
	require.NoError(t, err)
	_, err = c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: read.Revision, Subject: externalapi.NewOptString("Second")}, externalapi.TemplatesUpdateParams{ID: id})

	refused := testhelper.ExternalRefusal(t, err, http.StatusConflict)
	assert.Equal(t, externalapi.ProblemCodeRevisionChanged, refused.Code.Value)
	stored := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)
	assert.Equal(t, "First", stored.Subject)
	assert.EqualValues(t, first.Revision, stored.Revision)
}

// /api requires the revision as /site does: an update without one is refused and writes nothing.
func TestExternalTemplateUpdateWithoutARevisionIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	bearer := env.ScopedBearer(t, "templates:read", "templates:write")
	c := env.ExternalVia(t, bearer, testhelper.DropField("revision"))
	ctx := context.Background()
	before := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)

	_, err := c.TemplatesUpdate(ctx, &externalapi.UpdateTemplateInput{Revision: externalapi.Revision(before.Revision), Subject: externalapi.NewOptString("Blind")},
		externalapi.TemplatesUpdateParams{ID: entityIDString(fixtures.TemplateWelcomeID)})

	testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
	after := env.DB.EmailTemplate.GetX(ctx, fixtures.TemplateWelcomeID)
	assert.Equal(t, before.Subject, after.Subject)
	assert.Equal(t, before.Revision, after.Revision)
}
