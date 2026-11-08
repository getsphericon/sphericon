package users_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
)

func TestSecondFactorUnconfirmedSecretsCannotVerifyOrRegenerate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		secret := rapid.SampledFrom([]string{"", "sealed pending secret"}).Draw(t, "unconfirmed factor")
		trigger := rapid.SampledFrom([]users.SecondFactorTrigger{users.FactorVerify, users.FactorRegenerate, users.FactorDisable}).Draw(t, "active-only operation")
		before := &ent.User{SecondFactorSecretEncrypted: secret}
		stored := users.SecondFactorState("")
		machine := users.SecondFactorMachine(before, users.SecondFactorChange{Trigger: trigger, Step: 1}, func(_ context.Context, to users.SecondFactorState) (bool, error) {
			stored = to
			return true, nil
		})

		err := machine.FireCtx(t.Context(), trigger)

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Empty(t, stored)
	})
}

func TestSecondFactorConfirmationAlwaysRequiresANewerStep(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		previous := rapid.Int64Range(1, 1000000).Draw(t, "accepted step")
		step := rapid.Int64Range(0, previous).Draw(t, "replayed or older step")
		before := &ent.User{SecondFactorSecretEncrypted: "sealed pending secret", SecondFactorLastStep: previous}
		stored := users.SecondFactorState("")
		machine := users.SecondFactorMachine(before, users.SecondFactorChange{Trigger: users.FactorConfirm, Step: step}, func(_ context.Context, to users.SecondFactorState) (bool, error) {
			stored = to
			return true, nil
		})

		err := machine.FireCtx(t.Context(), users.FactorConfirm)

		require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		assert.Empty(t, stored)
	})
}

func TestSecondFactorInternalTransitionsCannotSkipTheirConditionalWrite(t *testing.T) {
	now := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		before *ent.User
		change users.SecondFactorChange
	}{
		{"replace pending", &ent.User{SecondFactorSecretEncrypted: "sealed"}, users.SecondFactorChange{Trigger: users.FactorStart, SealedSecret: "replacement"}},
		{"verify totp", &ent.User{SecondFactorSecretEncrypted: "sealed", SecondFactorConfirmedAt: &now}, users.SecondFactorChange{Trigger: users.FactorVerify, Step: 1}},
		{"verify recovery", &ent.User{SecondFactorSecretEncrypted: "sealed", SecondFactorConfirmedAt: &now}, users.SecondFactorChange{Trigger: users.FactorVerify}},
		{"regenerate", &ent.User{SecondFactorSecretEncrypted: "sealed", SecondFactorConfirmedAt: &now}, users.SecondFactorChange{Trigger: users.FactorRegenerate}},
		{"reset passkeys", &ent.User{}, users.SecondFactorChange{Trigger: users.FactorReset}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			machine := users.SecondFactorMachine(tc.before, tc.change, func(context.Context, users.SecondFactorState) (bool, error) { return false, nil })

			err := machine.FireCtx(t.Context(), tc.change.Trigger)

			require.ErrorIs(t, err, lifecycle.ErrInvalidTransition)
		})
	}
}
