package testhelper

import (
	"context"
	"time"
)

// ClaimOutbound moves the claim of a fixture Outbound message to `at`: a pending fixture message
// holds a claim long past, so a test that needs a live claim (a send still in flight) moves it
// instead of creating a message.
func (env *TestEnv) ClaimOutbound(ctx context.Context, workspaceID, messageID int64, at time.Time) {
	if err := env.DB.Scoped(workspaceID).OutboundMessage().UpdateOneID(messageID).SetClaimedAt(at).Exec(ctx); err != nil {
		panic(err)
	}
}
