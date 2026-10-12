package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/config"
	"github.com/stretchr/testify/require"
)

// ScratchDatabaseURL creates an empty, uniquely named database on the test
// server and returns its URL, dropping it when the test ends. For code that must
// run against a database with no schema at all (migrations on a fresh instance),
// which the shared, pre-migrated test database cannot provide.
func ScratchDatabaseURL(t *testing.T) string {
	t.Helper()
	cfg, err := config.Load("test")
	require.NoError(t, err)

	admin, err := sql.Open("pgx", cfg.DatabaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })

	name := "scratch_" + strings.NewReplacer("/", "_", "-", "_", " ", "_").Replace(strings.ToLower(t.Name()))
	if len(name) > 60 {
		name = name[:60]
	}
	ctx := context.Background()
	_, err = admin.ExecContext(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	require.NoError(t, err)
	_, err = admin.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %q`, name))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, name))
	})

	u, err := url.Parse(cfg.DatabaseURL)
	require.NoError(t, err)
	u.Path = "/" + name
	return u.String()
}
