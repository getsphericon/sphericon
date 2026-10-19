package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Content that cannot render is refused where it is saved, with the reason (ADR 0040).
func TestExternalRefusesContentThatCannotRender(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:write", "broadcasts:write")
	ctx := context.Background()

	tpl, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Broken", Body: externalapi.NewOptString("<p>{% if %}</p>"),
	})
	require.NoError(t, err)
	refused, ok := tpl.(*externalapi.TemplatesCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", tpl)
	assert.Contains(t, refused.Detail.Or(""), "render body")

	bc, err := c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name: "Broken", Body: externalapi.NewOptString("<table><tr><td>hi</td></tr>"),
	})
	require.NoError(t, err)
	refusedBroadcast, ok := bc.(*externalapi.BroadcastsCreateUnprocessableEntity)
	require.Truef(t, ok, "got %T", bc)
	assert.Contains(t, refusedBroadcast.Detail.Or(""), "<table> is never closed")
}
