package db_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The project carries no legacy (ADR 0047): a database holding an Order credited before the credit
// named the clicked message is refused before the credit check changes, rather than failing it.
func TestTheCreditedMessageMigrationRefusesADatabaseWithCreditedOrders(t *testing.T) {
	sqlDB := testhelper.SchemaBefore(t, "order_credited_message", filepath.Join("testdata", "order_credited_message"))

	_, err := db.Migrate(t.Context(), sqlDB)

	require.ErrorContains(t, err, "reset it, the project carries no legacy")
}
