package site

import (
	"context"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteSchemaApplyMigrations runs the pending migrations, the button of the error page of a
// development server (ADR 0033). A server that does not offer it answers 404.
func (h *Handlers) SiteSchemaApplyMigrations(ctx context.Context) (siteapi.SiteSchemaApplyMigrationsRes, error) {
	if h.migrations == nil {
		v := problem(http.StatusNotFound, "this server does not run migrations from the page")
		return &v, nil
	}
	if _, err := h.migrations.Apply(ctx); err != nil {
		return nil, err
	}
	return &siteapi.SiteSchemaApplyMigrationsNoContent{}, nil
}
