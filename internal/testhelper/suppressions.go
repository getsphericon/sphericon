package testhelper

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
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

// SuppressNumber is SuppressDestination for a phone number on sms.
func (env *TestEnv) SuppressNumber(ctx context.Context, number string) {
	err := env.DB.Scoped(fixtures.AcmeID).Suppression().UpdateOneID(fixtures.SuppressionLiftedID).
		SetChannel(channels.SMS).SetDestination(number).ClearLiftedAt().Exec(ctx)
	if err != nil {
		panic(err)
	}
}

// UnsubscribeFromBroadcasts opts a destination of Acme out of Broadcasts by moving the fixture's
// per-Automation opt-out onto it: a test that needs an opt-out landing at a chosen moment moves
// that row instead of creating one.
func (env *TestEnv) UnsubscribeFromBroadcasts(ctx context.Context, destination string) {
	err := env.DB.Scoped(fixtures.AcmeID).Unsubscribe().UpdateOneID(fixtures.UnsubscribeAutomationLeoID).
		SetDestination(destination).SetSendingSource(eligibility.SourceBroadcasts).Exec(ctx)
	if err != nil {
		panic(err)
	}
}

// UnsubscribeFromEverything opts a destination of Acme out of everything on email by moving the
// fixture's opt-out of everything that has no Contact onto it: a test that needs one landing at a
// chosen moment moves that row instead of creating one.
func (env *TestEnv) UnsubscribeFromEverything(ctx context.Context, destination string) {
	err := env.DB.Scoped(fixtures.AcmeID).Unsubscribe().UpdateOneID(fixtures.UnsubscribeEverythingNoContactID).
		SetDestination(destination).Exec(ctx)
	if err != nil {
		panic(err)
	}
}
