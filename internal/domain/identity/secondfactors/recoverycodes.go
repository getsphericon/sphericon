package secondfactors

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/recoverycode"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

// The unique constraints of the tables this package writes (ADR 0106).
var _ = db.Declare(
	db.Fault("recoverycode_user_id_code_hash", "the codes are drawn at random, and replaceCodesIn deletes the User's set before it writes the next"),
	db.Fault("second_factors_user_id_key", "the first enrollment inserts the factor row ON CONFLICT (user_id) DO NOTHING, and a concurrent one that finds it fails as a stale change"),
)

// RecoveryCodeCount is the size of a Recovery code set.
const RecoveryCodeCount = 10

// unusedCodes counts the User's Recovery codes not spent yet.
func unusedCodes(ctx context.Context, client *ent.Client, userID int64) (int, error) {
	return client.RecoveryCode.Query().
		Where(recoverycode.UserID(userID), recoverycode.UsedAtIsNil()).
		Count(ctx)
}

// replaceCodesIn deletes the User's Recovery codes and stores a fresh set on the transaction tx,
// returning its plaintext, after the factor's move that allows a new set.
func replaceCodesIn(ctx context.Context, tx *ent.Client, userID int64) ([]string, error) {
	if err := deleteCodesIn(ctx, tx, userID); err != nil {
		return nil, err
	}
	codes := make([]string, RecoveryCodeCount)
	builders := make([]*ent.RecoveryCodeCreate, RecoveryCodeCount)
	for i := range codes {
		codes[i] = newCode()
		builders[i] = tx.RecoveryCode.Create().SetUserID(userID).SetCodeHash(credentials.HashRecoveryCode(codes[i]))
	}
	if err := tx.RecoveryCode.CreateBulk(builders...).Exec(ctx); err != nil {
		return nil, err
	}
	return codes, nil
}

// deleteCodesIn deletes every Recovery code of the User on the transaction tx, when the factor goes.
func deleteCodesIn(ctx context.Context, tx *ent.Client, userID int64) error {
	_, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx)
	return err
}

// spendCodeIn spends code, an unused Recovery code of the User, at, on the transaction tx; false
// when code is none of them (or already spent). Case and spacing do not matter.
func spendCodeIn(ctx context.Context, tx *ent.Client, userID int64, code string, at time.Time) (bool, error) {
	err := lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		n, err := tx.RecoveryCode.Update().
			Where(
				recoverycode.UserID(userID),
				recoverycode.CodeHash(credentials.HashRecoveryCode(code)),
				recoverycode.UsedAtIsNil(),
			).
			SetUsedAt(at).
			Save(ctx)
		return n == 1, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return false, nil
	}
	return err == nil, err
}

// newCode is ten random base32 characters (50 bits) as two groups of five.
func newCode() string {
	s := strings.ToLower(rand.Text()[:10])
	return s[:5] + "-" + s[5:]
}
