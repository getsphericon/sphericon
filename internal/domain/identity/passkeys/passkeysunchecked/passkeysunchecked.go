// Package passkeysunchecked holds the passkeys module's unchecked removal (ADR 0110): every Passkey
// of a User goes only in a Second factor reset, which checks who resets and that there is something
// to clear, clears the Second factor with them and records who reset, so the check is the use case
// internal/usecase/secondfactorreset's (ADR 0020, ADR 0052); only a use case may import this
// package. The passkeys module does not import it.
package passkeysunchecked

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/passkey"
)

// DeleteAllIn deletes every Passkey of the User on the transaction tx. Only a Second factor reset
// calls it (internal/usecase/secondfactorreset), which checks who resets first and ends the
// User's sessions in the same transaction.
func DeleteAllIn(ctx context.Context, tx *ent.Client, userID int64) error {
	_, err := tx.Passkey.Delete().Where(passkey.UserID(userID)).Exec(ctx)
	return err
}
