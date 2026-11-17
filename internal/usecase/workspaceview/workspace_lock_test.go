package workspaceview_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

// Clearing the postal address and scheduling an email Broadcast both take the Workspace's lock
// (ADR 0099, ADR 0156), so the second waits for the first and reads what it committed: an email is
// never armed under an address cleared meanwhile.
func TestAClearingOfThePostalAddressQueuedFirstRefusesTheSchedule(t *testing.T) {
	r := newPostalAddressRace(t)

	errs := r.env.QueueBehindWorkspaceLock(t, r.scope, r.clear, r.schedule)

	require.NoError(t, errs["clear"])
	require.ErrorIs(t, errs["schedule"], outboundmessages.ErrPostalAddressRequired)
	status := r.env.DB.Broadcast.GetX(t.Context(), fixtures.BroadcastGlobexID).Status
	assert.Equal(t, broadcast.StatusDraft, status)
}

func TestAScheduleQueuedFirstRefusesTheClearingOfThePostalAddress(t *testing.T) {
	r := newPostalAddressRace(t)

	errs := r.env.QueueBehindWorkspaceLock(t, r.scope, r.schedule, r.clear)

	require.NoError(t, errs["schedule"])
	var used *liveuse.Error
	require.ErrorAs(t, errs["clear"], &used)
	assert.Equal(t, []liveuse.User{{Kind: liveuse.Broadcast, ID: fixtures.BroadcastGlobexID, Name: fixtures.BroadcastGlobexName}}, used.Users)
	address := r.env.DB.Workspace.GetX(t.Context(), fixtures.GlobexID).PostalAddress
	assert.NotEmpty(t, address)
}

// postalAddressRace is Globex's email draft sending through its SMTP Integration, with a writer
// that clears the Workspace's postal address as its Owner Jane does and one that schedules the
// draft, on a database of its own. Globex has nothing else armed.
type postalAddressRace struct {
	env             *testhelper.TestEnv
	scope           *ent.Scoped
	clear, schedule testhelper.Writer
}

func newPostalAddressRace(t *testing.T) postalAddressRace {
	t.Helper()
	ctx := context.Background()
	env := testhelper.Setup(t, testhelper.Committed())
	member, err := testhelper.Module[*memberships.Module](env).Scope(ctx, fixtures.OwnerJaneID, fixtures.GlobexPublicID)
	require.NoError(t, err)
	s, m := member.Scope, member.Membership
	sends := testhelper.Module[*broadcasts.Module](env)
	view := testhelper.Module[*workspaceview.UseCase](env)
	_, err = sends.Update(ctx, s, fixtures.BroadcastGlobexID, broadcasts.UpdateInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastGlobexID), IntegrationID: patch.Set[int64](fixtures.IntegrationGlobexID)})
	require.NoError(t, err)
	empty := ""
	return postalAddressRace{
		env:   env,
		scope: s,
		clear: testhelper.Writer{Name: "clear", Run: func() error {
			ws := env.DB.Workspace.GetX(ctx, fixtures.GlobexID)
			_, err := view.Update(ctx, s, workspaceview.Caller{Membership: m}, workspaceview.UpdateInput{Revision: ws.Revision, Name: ws.Name, PostalAddress: &empty})
			return err
		}},
		schedule: testhelper.Writer{Name: "schedule", Run: func() error {
			_, err := sends.Schedule(ctx, s, fixtures.BroadcastGlobexID, broadcasts.ScheduleInput{Revision: testhelper.BroadcastRevisionIn(ctx, t, s, fixtures.BroadcastGlobexID), ScheduledAt: time.Now().Add(time.Hour)})
			return err
		}},
	}
}
