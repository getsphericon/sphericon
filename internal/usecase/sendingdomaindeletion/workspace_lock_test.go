package sendingdomaindeletion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/removalcheck"
	"github.com/getsphericon/sphericon/internal/domain/sending/holds"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/registry"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/sendingdomaindeletion"
)

// anySender resolves every Integration to a sender that accepts every message.
type anySender struct{}

func (anySender) EmailSender(context.Context, *ent.Scoped, *ent.Integration) (messaging.EmailSender, error) {
	return anySender{}, nil
}

func (anySender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, nil
}

// Deleting a Sending domain and scheduling a draft that sends from an address on it take the
// Workspace's lock (ADR 0099), so the delete queued behind the schedule reads the armed Broadcast
// and is refused naming it: the domain is never deleted under a Broadcast the check found a draft.
func TestAScheduleQueuedFirstRefusesTheDeleteOfTheDomainItSendsFrom(t *testing.T) {
	ctx := context.Background()
	race := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	s, bus := race.Scope, race.Bus
	cipher := testhelper.Cipher(t)
	out := outboundmessages.New(bus, holds.New(func() bool { return true }, time.Now), anySender{}, messagetoken.New("test-secret", "http://local", time.Now), time.Now)
	sends := broadcasts.New(bus, testhelper.Renderer(), out)
	domains := sendingdomains.New(bus, cipher, nil, time.Now)
	deletion := sendingdomaindeletion.New(bus, domains, removalcheck.New(messaging.NewResolver(cipher, registry.Default())), time.Now)
	from := broadcasts.EmailContentPatch{FromEmail: patch.Set("news@" + fixtures.SendingDomainVerifiedDomain)}
	_, err := sends.Update(ctx, s, fixtures.BroadcastDraftID, broadcasts.UpdateInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), Content: from})
	require.NoError(t, err)
	schedule := testhelper.Writer{Name: "schedule", Run: func() error {
		_, err := sends.Schedule(ctx, s, fixtures.BroadcastDraftID, testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastDraftID), time.Now().Add(time.Hour))
		return err
	}}
	remove := testhelper.Writer{Name: "delete", Run: func() error { return deletion.Delete(ctx, s, fixtures.SendingDomainVerifiedID) }}

	errs := race.Queue(t, schedule, remove)

	require.NoError(t, errs["schedule"])
	b, err := sends.Get(ctx, s, fixtures.BroadcastDraftID)
	require.NoError(t, err)
	var used *liveuse.Error
	require.ErrorAs(t, errs["delete"], &used)
	assert.Equal(t, []liveuse.User{{Kind: liveuse.Broadcast, ID: b.ID, Name: b.Name}}, used.Users)
	kept, err := domains.Get(ctx, s, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.Nil(t, kept.DiscardedAt, "the domain the Broadcast sends from is kept")
}
