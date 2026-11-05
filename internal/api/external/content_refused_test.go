package external_test

import (
	"context"
	"net/http"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
)

// Content that cannot render is refused where it is saved, with the reason (ADR 0040).
func TestExternalRefusesContentThatCannotRender(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:write", "broadcasts:write")
	ctx := context.Background()

	_, err := c.TemplatesCreate(ctx, &externalapi.CreateTemplateInput{
		Name: "Broken", Body: externalapi.NewOptString("<p>{% if %}</p>"),
	})
	refused := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refused.Detail.Or(""), "render body")

	_, err = c.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:    "Broken",
		Content: testhelper.ExternalEmail(externalapi.EmailBroadcastContentInput{Body: externalapi.NewOptString("<table><tr><td>hi</td></tr>")}),
	})
	refusedBroadcast := testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
	assert.Contains(t, refusedBroadcast.Detail.Or(""), "<table> is never closed")
}
