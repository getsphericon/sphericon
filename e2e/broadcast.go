//go:build e2e

package e2e

import (
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
)

// Broadcast is what a scenario chooses about a Broadcast; the rest defaults.
type Broadcast struct {
	Name    string
	Subject string
	// Body is MJML (the product's one body format).
	Body string
}

// SendBroadcast creates a Broadcast from the Workspace's FromEmail/FromName, sets its
// audience to all active Contacts and schedules it for now. The send itself is an
// asynchronous job: observe it with Inbox.Wait.
func (w *Workspace) SendBroadcast(b Broadcast) {
	w.t.Helper()
	ctx := w.t.Context()
	created := w.createBroadcast(b)

	aud, err := w.api.BroadcastsSetAudience(ctx, &externalapi.SetBroadcastAudienceInput{SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetAudienceParams{ID: created.ID})
	ok[externalapi.BroadcastResource](w.t, "set audience", aud, err)

	sched, err := w.api.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now())},
		externalapi.BroadcastsScheduleParams{ID: created.ID})
	ok[externalapi.BroadcastResource](w.t, "schedule broadcast", sched, err)
}

// createBroadcast creates a draft Broadcast from the Workspace's FromEmail/FromName.
func (w *Workspace) createBroadcast(b Broadcast) *externalapi.BroadcastResource {
	w.t.Helper()
	if b.Name == "" {
		b.Name = "e2e broadcast " + uniq()
	}
	res, err := w.api.BroadcastsCreate(w.t.Context(), &externalapi.CreateBroadcastInput{
		Name:      b.Name,
		Subject:   externalapi.NewOptString(b.Subject),
		Body:      externalapi.NewOptString(b.Body),
		FromName:  externalapi.NewOptString(w.FromName),
		FromEmail: externalapi.NewOptEmailAddress(externalapi.EmailAddress(w.FromEmail)),
	})
	return ok[externalapi.BroadcastResource](w.t, "create broadcast", res, err)
}
