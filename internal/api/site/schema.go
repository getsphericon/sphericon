package site

import (
	"context"
	"github.com/getsphericon/sphericon/internal/problem"
	"net/http"
)

// SiteSchemaApplyMigrations runs the pending migrations, the button of the error page of a
// development server (ADR 0033). A server that does not offer it answers 404.
func (h *Handlers) SiteSchemaApplyMigrations(ctx context.Context) error {
	if h.migrations == nil {
		// Kept by hand: a development server's page, outside the contract's refusals.
		// ast-grep-ignore: no-hand-written-problem-in-handlers
		return &problem.Error{Status: http.StatusNotFound, Detail: "this server does not run migrations from the page"}
	}
	if _, err := h.migrations.Apply(ctx); err != nil {
		return err
	}
	return nil
}
