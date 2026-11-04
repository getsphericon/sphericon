package testhelper

import (
	"context"

	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	"github.com/getsphericon/sphericon/internal/eligibility"
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

// SuppressNumber is SuppressDestination for a phone number on sms.
func (env *TestEnv) SuppressNumber(ctx context.Context, number string) {
	err := env.DB.Scoped(fixtures.AcmeID).Suppression().UpdateOneID(fixtures.SuppressionLiftedID).
		SetChannel(suppression.ChannelSms).SetDestination(number).ClearLiftedAt().Exec(ctx)
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

// UnsubscribeTelegramFromBroadcasts is UnsubscribeFromBroadcasts for a Telegram user id: the
// person's stop, which silences every bot of the Workspace for them.
func (env *TestEnv) UnsubscribeTelegramFromBroadcasts(ctx context.Context, userID string) {
	err := env.DB.Scoped(fixtures.AcmeID).Unsubscribe().UpdateOneID(fixtures.UnsubscribeAutomationLeoID).
		SetChannel(unsubscribe.ChannelTelegram).SetDestination(userID).SetSendingSource(eligibility.SourceBroadcasts).Exec(ctx)
	if err != nil {
		panic(err)
	}
}
