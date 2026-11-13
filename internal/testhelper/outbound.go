package testhelper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
)

// OutboundModule is the Outbound module the harness's own sends run through, for a test that
// sends from a surface the API does not expose (an Automation step).
func (env *TestEnv) OutboundModule() *outboundmessages.Module { return env.sender }

// ClaimOutbound moves the claim of a fixture Outbound message to `at`: a pending fixture message
// holds a claim long past, so a test that needs a live claim (a send still in flight) moves it
// instead of creating a message.
func (env *TestEnv) ClaimOutbound(ctx context.Context, workspaceID, messageID int64, at time.Time) {
	if err := env.DB.Scoped(workspaceID).OutboundMessage().UpdateOneID(messageID).SetClaimedAt(at).Exec(ctx); err != nil {
		panic(err)
	}
}

// Tracked is what an open or a click token names for the Outbound message id: the message and its
// Workspace, read from its row.
func (env *TestEnv) Tracked(tb testing.TB, messageID int64) messagetoken.Message {
	tb.Helper()
	msg, err := env.DB.OutboundMessage.Get(context.Background(), messageID)
	require.NoError(tb, err)
	return messagetoken.Message{WorkspaceID: msg.WorkspaceID, ID: msg.ID}
}
