//go:build e2e

package e2e

import (
	"encoding/json"
	"time"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/broadcasts"
)

// Broadcast is what a scenario chooses about a Broadcast; the rest defaults.
type Broadcast struct {
	Name    string
	Subject string
	// Body is HTML (the product's one body format).
	Body string
	// Doc is the editor's JSON document; the server builds the body from it, so a Broadcast
	// carries a Body or a Doc, not both.
	Doc json.RawMessage
}

// SendBroadcast creates a Broadcast from the Workspace's FromEmail/FromName, sets its
// audience to all active Contacts and schedules it for now. The send itself is an
// asynchronous job: observe it with Inbox.Wait.
func (w *Workspace) SendBroadcast(b Broadcast) {
	w.t.Helper()
	ctx := w.t.Context()
	if b.Name == "" {
		b.Name = "e2e broadcast " + uniq()
	}
	id := w.createBroadcast(b)

	aud, err := w.api.BroadcastsSetAudience(ctx, &externalapi.SetBroadcastAudienceInput{SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetAudienceParams{ID: id})
	ok[externalapi.BroadcastResource](w.t, "set audience", aud, err)

	sched, err := w.api.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now())},
		externalapi.BroadcastsScheduleParams{ID: id})
	ok[externalapi.BroadcastResource](w.t, "schedule broadcast", sched, err)
}

// createBroadcast makes the draft. A Body goes through the external API; a Doc is written the way
// the editor's save does (the external API accepts only HTML), through the Broadcasts module on
// the Workspace's scope, which renders the document to the stored body.
func (w *Workspace) createBroadcast(b Broadcast) externalapi.EntityId {
	w.t.Helper()
	ctx := w.t.Context()
	if len(b.Doc) > 0 {
		created, err := broadcasts.New(nil).Create(ctx, w.scoped, broadcasts.Fields{
			Name:      &b.Name,
			Subject:   &b.Subject,
			FromName:  &w.FromName,
			FromEmail: &w.FromEmail,
			Doc:       b.Doc,
		})
		require.NoError(w.t, err, "create broadcast from a document")
		return externalapi.EntityId(created.ID)
	}
	res, err := w.api.BroadcastsCreate(ctx, &externalapi.CreateBroadcastInput{
		Name:      b.Name,
		Subject:   externalapi.NewOptString(b.Subject),
		Body:      externalapi.NewOptString(b.Body),
		FromName:  externalapi.NewOptString(w.FromName),
		FromEmail: externalapi.NewOptEmailAddress(externalapi.EmailAddress(w.FromEmail)),
	})
	return ok[externalapi.BroadcastResource](w.t, "create broadcast", res, err).ID
}
