package testhelper

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/internal/outbound"
)

// OutboundModule is the Outbound module the harness's own sends run through, for a test that
// sends from a surface the API does not expose (an Automation step).
func (env *TestEnv) OutboundModule() *outbound.Module { return env.sender }

// ClaimOutbound moves the claim of a fixture Outbound message to `at`: a pending fixture message
// holds a claim long past, so a test that needs a live claim (a send still in flight) moves it
// instead of creating a message.
func (env *TestEnv) ClaimOutbound(ctx context.Context, workspaceID, messageID int64, at time.Time) {
	if err := env.DB.Scoped(workspaceID).OutboundMessage().UpdateOneID(messageID).SetClaimedAt(at).Exec(ctx); err != nil {
		panic(err)
	}
}
