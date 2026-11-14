package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// defaultAfterMigrating is the default Send cap suspension reads once the migrations after the
// fixtures of dir have run.
func defaultAfterMigrating(t *testing.T, dir string) *int {
	t.Helper()
	sqlDB := testhelper.SchemaBefore(t, "default_send_cap", filepath.Join("testdata", dir))
	_, err := db.Migrate(t.Context(), sqlDB)
	require.NoError(t, err)
	perDay, _, err := suspension.NewDefaultSendCap(entclient.Core.New(sqlDB), true).Get(t.Context())
	require.NoError(t, err)
	return perDay
}

// The default Send cap moves from the Instance state row to suspension's table (ADR 0146): a
// stored default is kept, and an Instance that had none still has none.
func TestTheDefaultSendCapMigrationKeepsAStoredDefault(t *testing.T) {
	stored := defaultAfterMigrating(t, "default_send_cap")
	none := defaultAfterMigrating(t, "default_send_cap_none")

	assert.Equal(t, new(777), stored)
	assert.Nil(t, none)
}
