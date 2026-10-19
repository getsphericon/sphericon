package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The external API sanitizes a body where it is saved, like the site API (ADR 0040).
func TestExternalSanitizesBodiesOnSave(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:write", "broadcasts:write")
	ctx := context.Background()
	hostile := `<p onclick="steal()">Hi {{ first_name }}</p><script>alert(1)</script>`

	tpl, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Hostile", Body: externalapi.NewOptString(hostile),
	})
	require.NoError(t, err)
	created, ok := tpl.(*externalapi.TemplateResource)
	require.Truef(t, ok, "got %T", tpl)
	assert.Equal(t, `<p>Hi {{ first_name }}</p>`, created.Body)

	bc, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name: "Hostile", Body: externalapi.NewOptString(hostile),
	})
	require.NoError(t, err)
	broadcast, ok := bc.(*externalapi.BroadcastResource)
	require.Truef(t, ok, "got %T", bc)
	assert.Equal(t, `<p>Hi {{ first_name }}</p>`, broadcast.Body)
}
