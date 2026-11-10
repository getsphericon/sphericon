package external_test

import (
	"context"
	"net/http"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Mapping only: the blank-name rule itself is tested at the templates module.
func TestExternalTemplatesMapABlankNameTo422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:read", "templates:write")

	_, err := c.TemplatesCreate(context.Background(), &externalapi.CreateTemplateInput{Name: ""})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestExternalTemplatesUpdateMapsABlankNameTo422(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "templates:read", "templates:write")
	tmpl := variablesTemplate(env.DB)

	_, err := c.TemplatesUpdate(context.Background(), &externalapi.UpdateTemplateInput{Name: externalapi.NewOptName("")},
		externalapi.TemplatesUpdateParams{ID: templateID(tmpl)})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}
