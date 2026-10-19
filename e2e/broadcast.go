//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/go-faster/jx"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
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
	if b.Name == "" {
		b.Name = "e2e broadcast " + uniq()
	}
	w.ActivateBroadcast(w.createBroadcast(b))
}

// ActivateBroadcast is a person's activation of a draft: it sets the audience to all active
// Contacts and schedules the Broadcast for now, with the Workspace's own token, which carries the
// send scope an agent's token does not.
func (w *Workspace) ActivateBroadcast(id externalapi.EntityId) {
	w.t.Helper()
	ctx := w.t.Context()
	aud, err := w.api.BroadcastsSetAudience(ctx, &externalapi.SetBroadcastAudienceInput{SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetAudienceParams{ID: id})
	ok[externalapi.BroadcastResource](w.t, "set audience", aud, err)

	sched, err := w.api.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{ScheduledAt: externalapi.Timestamp(time.Now())},
		externalapi.BroadcastsScheduleParams{ID: id})
	ok[externalapi.BroadcastResource](w.t, "schedule broadcast", sched, err)
}

// createBroadcast makes the draft through the external API, from the Body or the Doc.
func (w *Workspace) createBroadcast(b Broadcast) externalapi.EntityId {
	w.t.Helper()
	in := &externalapi.CreateBroadcastInput{
		Name:      b.Name,
		Subject:   externalapi.NewOptString(b.Subject),
		FromName:  externalapi.NewOptString(w.FromName),
		FromEmail: externalapi.NewOptEmailAddress(externalapi.EmailAddress(w.FromEmail)),
	}
	if len(b.Doc) > 0 {
		in.Doc = externalapi.NewOptCreateBroadcastInputDoc(docObject[externalapi.CreateBroadcastInputDoc](w.t, b.Doc))
	} else {
		in.Body = externalapi.NewOptString(b.Body)
	}
	res, err := w.api.BroadcastsCreate(w.t.Context(), in)
	return ok[externalapi.BroadcastResource](w.t, "create broadcast", res, err).ID
}

// docObject is a document's JSON as the opaque object the external API takes.
func docObject[M ~map[string]jx.Raw](t testing.TB, doc json.RawMessage) M {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(doc, &fields), "the document is a JSON object")
	out := make(M, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return out
}
