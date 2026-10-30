package userfeed_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/consentrecord"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/userfeed"
)

// A position taken outside a transaction would be a second statement: a crash between the two
// loses the change, and two of them can commit out of order. The plain client refuses such a
// change, so every path goes through a transaction; a Consent record is refused whatever its
// item, so a writer that forgets the transaction is caught on its first record.
func TestPlainClientRefusesAChangeToTheUserFeed(t *testing.T) {
	// A database of the test's own: a refusal that regressed would commit.
	client := db.NewEntClient(testhelper.RealDB(t))

	// test writes: the hook fires on this write
	_, err := client.User.Create().SetName("Plain").SetEmail("plain@plain.test").Save(t.Context())
	require.ErrorIs(t, err, userfeed.ErrNeedsTransaction)

	err = client.User.UpdateOneID(fixtures.OutsiderOscarID).SetName("Renamed").Exec(t.Context())
	require.ErrorIs(t, err, userfeed.ErrNeedsTransaction)

	err = client.User.DeleteOneID(fixtures.OutsiderOscarID).Exec(t.Context())
	require.ErrorIs(t, err, userfeed.ErrNeedsTransaction)

	// test writes: the hook fires on this write
	_, err = client.ConsentRecord.Create().
		SetUserID(fixtures.OutsiderOscarID).SetVersionID(fixtures.ServiceTermsCurrentID).
		SetDecision(consentrecord.DecisionAccepted).SetSource(consentrecord.SourceAccountSettings).
		Save(t.Context())
	require.ErrorIs(t, err, userfeed.ErrNeedsTransaction)

	err = client.SignupItem.UpdateOneID(fixtures.OperatorNewsID).SetMailingsConsent(true).Exec(t.Context())
	require.ErrorIs(t, err, userfeed.ErrNeedsTransaction)
}
