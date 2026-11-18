package bindings_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses a Telegram Binding without the chat its messages go to, whatever code
// writes it: a Binding's columns are its Channel's (bindings_destination_check).
func TestTheDatabaseRefusesATelegramBindingWithoutAChat(t *testing.T) {
	env := testhelper.Setup(t)
	// test writes: the refused insert is the operation under test.
	err := env.DB.Binding.Create().SetWorkspaceID(fixtures.AcmeID).SetIntegrationID(fixtures.IntegrationAcmeTelegramID).
		SetChannel(binding.ChannelTelegram).SetDestination("424242").SetContactID(fixtures.ContactAliceID).
		SetBoundAt(time.Now()).Exec(t.Context())
	require.Error(t, err)
	assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "bindings_destination_check", pgErr.ConstraintName)
}
