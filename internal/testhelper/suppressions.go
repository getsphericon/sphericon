package testhelper

import (
	"context"

	"github.com/getsphericon/sphericon/internal/fixtures"
)

// SuppressDestination puts a destination of Acme on the Suppression list by re-opening the
// fixture's lifted Suppression for it: a test that needs a Suppression landing at a chosen moment
// reopens that row instead of creating one.
func (env *TestEnv) SuppressDestination(ctx context.Context, destination string) {
	err := env.DB.Scoped(fixtures.AcmeID).Suppression().UpdateOneID(fixtures.SuppressionLiftedID).
		SetDestination(destination).ClearLiftedAt().Exec(ctx)
	if err != nil {
		panic(err)
	}
}
