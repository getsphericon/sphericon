package testhelper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/events"
)

// BackdateEvent moves the stored Event of a published envelope to at, for a test that needs an
// Event of the past: an event is stored when it is published, at the time its producer gives,
// and most producers give none. occurred_at never changes on a row, so the row is replaced by a
// copy at that time.
func (env *TestEnv) BackdateEvent(t *testing.T, published events.Envelope, at time.Time) {
	t.Helper()
	ctx := t.Context()
	stored, err := env.DB.Event.Query().Where(event.SourceID(published.SourceID())).Only(ctx)
	require.NoError(t, err, "the envelope's Event is stored")
	require.NoError(t, env.DB.Event.DeleteOne(stored).Exec(ctx))
	err = env.DB.Event.Create().
		SetWorkspaceID(stored.WorkspaceID).
		SetNillableSourceID(stored.SourceID).
		SetNillableContactID(stored.ContactID).
		SetNillableVisitorID(stored.VisitorID).
		SetSubjectID(stored.SubjectID).
		SetNillableEmail(stored.Email).
		SetNillablePhone(stored.Phone).
		SetAction(stored.Action).
		SetProperties(stored.Properties).
		SetOccurredAt(at).
		SetMachine(stored.Machine).
		SetCaptured(stored.Captured).
		SetNillableBroadcastID(stored.BroadcastID).
		SetNillableAutomationID(stored.AutomationID).
		SetNillableURL(stored.URL).
		SetBackfill(stored.Backfill).
		Exec(ctx)
	require.NoError(t, err)
}
