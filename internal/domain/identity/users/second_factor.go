package users

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/user"
)

// The User's TOTP Second factor (ADR 0020) is three columns of the User row: the sealed secret,
// the moment it was confirmed and the last time step accepted. These are their owner's writes.
// The checks around them (a code, an active or a pending factor) live in the use case
// internal/usecase/secondfactorenrolment, the only caller of the writes below; the session epoch
// they bump ends every session of the User.

// Issuer names the Instance to a User's authenticators: the TOTP app that holds the Second
// factor's secret, and the relying party of a Passkey.
const Issuer = "sphericon"

// SetPendingSecondFactor stores sealed as the User's pending Second factor secret, replacing an
// earlier pending one: it counts once ConfirmSecondFactorIn confirms it.
func SetPendingSecondFactor(ctx context.Context, client *ent.Client, userID int64, sealed string) error {
	return client.User.UpdateOneID(userID).
		SetSecondFactorSecretEncrypted(sealed).
		ClearSecondFactorConfirmedAt().
		SetSecondFactorLastStep(0).
		Exec(ctx)
}

// AcceptSecondFactorStepIn stores step as the User's last accepted TOTP time step on the
// transaction tx, when it is later than the stored one; false when it is not, so a code is
// never accepted twice. The conditional write makes two concurrent uses of one code accept only
// one.
func AcceptSecondFactorStepIn(ctx context.Context, tx *ent.Client, userID, step int64) (bool, error) {
	n, err := tx.User.Update().
		Where(user.ID(userID), user.SecondFactorLastStepLT(step)).
		SetSecondFactorLastStep(step).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// ConfirmSecondFactorIn confirms the User's pending Second factor as of confirmedAt and bumps the
// session epoch on the transaction tx, returning the stored User.
func ConfirmSecondFactorIn(ctx context.Context, tx *ent.Client, userID int64, confirmedAt time.Time) (*ent.User, error) {
	return tx.User.UpdateOneID(userID).SetSecondFactorConfirmedAt(confirmedAt).AddSessionEpoch(1).Save(ctx)
}

// ClearSecondFactorIn clears the User's Second factor and bumps the session epoch on the
// transaction tx, returning the stored User.
func ClearSecondFactorIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	return tx.User.UpdateOneID(userID).
		ClearSecondFactorSecretEncrypted().
		ClearSecondFactorConfirmedAt().
		SetSecondFactorLastStep(0).
		AddSessionEpoch(1).
		Save(ctx)
}

// BumpSessionEpochIn bumps the User's session epoch on the transaction tx, ending every session
// of theirs, and returns the stored User.
func BumpSessionEpochIn(ctx context.Context, tx *ent.Client, userID int64) (*ent.User, error) {
	return tx.User.UpdateOneID(userID).AddSessionEpoch(1).Save(ctx)
}
