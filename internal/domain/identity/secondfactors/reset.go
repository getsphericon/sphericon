package secondfactors

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
)

// EnrolledIn reports, on the transaction tx, whether the User has a Second factor, active or a
// pending enrollment: what a Second factor reset would clear besides the Passkeys.
func EnrolledIn(ctx context.Context, tx *ent.Client, userID int64) (bool, error) {
	f, err := factorOf(ctx, tx, userID)
	return stateOf(f) != FactorNone, err
}
