package testhelper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
	"github.com/getsphericon/sphericon/internal/domain/sending/frequencycap"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// OutboundModule is the Outbound module the harness's own sends run through, for a test that
// sends from a surface the API does not expose (an Automation step).
func (env *TestEnv) OutboundModule() *outboundmessages.Module {
	return Module[*outboundmessages.Module](env)
}

// OutboundModules are the Modules of an Outbound send module a test builds over bus and now: the
// env's Contacts (a send only reads through it), and the Chats, the Provider limit's reads, the
// Standing and the Frequency cap over that bus and clock, as the wiring builds them over the Instance's. A test with a
// bus or a clock of its own passes it, so the chats publish in its send's transaction.
func (env *TestEnv) OutboundModules(bus *events.Bus, now func() time.Time) outboundmessages.Modules {
	return outboundmessages.Modules{
		Contacts: Module[*contacts.Module](env),
		Chats:    bindings.NewChats(bus, now),
		Limits:   providerlimit.New(now),
		Standing: integrations.NewStanding(now),
		Consents: Module[*engagementtracking.Module](env),
		Caps:     frequencycap.New(bus, now),
	}
}

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
