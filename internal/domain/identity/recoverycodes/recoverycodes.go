// Package recoverycodes owns a User's Recovery codes (ADR 0020): a set of single-use codes
// issued with a Second factor, stored hashed and shown once. A Recovery code belongs to the
// User, not a Workspace, so the package works on the raw *ent.Client: its writes join the
// transaction of the Second factor's step that makes them (internal/usecase/secondfactorcheck),
// which checks the Second factor first.
package recoverycodes

import (
	"context"
	"crypto/rand"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/recoverycode"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
)

// Count is the size of a Recovery code set.
const Count = 10

// Unused counts the User's Recovery codes not spent yet.
func Unused(ctx context.Context, client *ent.Client, userID int64) (int, error) {
	return client.RecoveryCode.Query().
		Where(recoverycode.UserID(userID), recoverycode.UsedAtIsNil()).
		Count(ctx)
}

// ReplaceIn deletes the User's Recovery codes and stores a fresh set on the transaction tx,
// returning its plaintext. Only the Second factor's use case calls it, after it has checked
// that the User may have a new set.
func ReplaceIn(ctx context.Context, tx *ent.Client, userID int64) ([]string, error) {
	if err := DeleteIn(ctx, tx, userID); err != nil {
		return nil, err
	}
	codes := make([]string, Count)
	builders := make([]*ent.RecoveryCodeCreate, Count)
	for i := range codes {
		codes[i] = newCode()
		builders[i] = tx.RecoveryCode.Create().SetUserID(userID).SetCodeHash(credentials.HashRecoveryCode(codes[i]))
	}
	if err := tx.RecoveryCode.CreateBulk(builders...).Exec(ctx); err != nil {
		return nil, err
	}
	return codes, nil
}

// DeleteIn deletes every Recovery code of the User on the transaction tx. Only the Second
// factor's use case calls it, when the factor goes.
func DeleteIn(ctx context.Context, tx *ent.Client, userID int64) error {
	_, err := tx.RecoveryCode.Delete().Where(recoverycode.UserID(userID)).Exec(ctx)
	return err
}

// SpendIn spends code, an unused Recovery code of the User, at, on the transaction tx; false
// when code is none of them (or already spent). Case and spacing do not matter.
func SpendIn(ctx context.Context, tx *ent.Client, userID int64, code string, at time.Time) (bool, error) {
	n, err := tx.RecoveryCode.Update().
		Where(
			recoverycode.UserID(userID),
			recoverycode.CodeHash(credentials.HashRecoveryCode(code)),
			recoverycode.UsedAtIsNil(),
		).
		SetUsedAt(at).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// newCode is ten random base32 characters (50 bits) as two groups of five.
func newCode() string {
	s := strings.ToLower(rand.Text()[:10])
	return s[:5] + "-" + s[5:]
}
