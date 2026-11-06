package db

import (
	"context"
	"log/slog"
	"time"
)

// PageMigrations runs the migrations of a development server from its error page (ADR 0033,
// config.MigratesFromThePage): the server started on a database that has not run them, and
// SchemaGuard answers 503 until they run.
type PageMigrations struct {
	url     string
	timeout time.Duration
	watch   *SchemaWatch
}

// NewPageMigrations migrates the database at url within timeout (0 = no limit) and makes watch
// ask the database again once it has.
func NewPageMigrations(url string, timeout time.Duration, watch *SchemaWatch) *PageMigrations {
	return &PageMigrations{url: url, timeout: timeout, watch: watch}
}

// Apply runs what `sphericon migrate` runs and reports how many migrations ran. It ignores the
// cancellation of ctx, which a closed tab or the request timeout would bring halfway through a
// migration; the timeout bounds it instead.
func (m *PageMigrations) Apply(ctx context.Context) (int, error) {
	ctx = context.WithoutCancel(ctx)
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}
	applied, err := ApplyMigrations(ctx, m.url)
	m.watch.Forget()
	if err != nil {
		return 0, err
	}
	slog.Info("migrations applied from the error page", "count", applied)
	return applied, nil
}
