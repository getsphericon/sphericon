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
	// WithoutFrom leaves the From unset, as a Workspace with no verified Sending domain has to:
	// the Workspace's FromEmail is refused until its domain is verified.
	WithoutFrom bool
}

// SendBroadcast creates a Broadcast from the Workspace's FromEmail/FromName, sets its
// Segment to all Contacts and schedules it for now. The send itself is an
// asynchronous job: observe it with Inbox.Wait.
func (w *Workspace) SendBroadcast(b Broadcast) {
	w.t.Helper()
	w.ActivateBroadcast(w.createBroadcast(b))
}

// ActivateBroadcast is a person's activation of a draft: it sets the Segment to all
// Contacts and schedules the Broadcast for now, with the Workspace's own token, which carries the
// send scope an agent's token does not.
func (w *Workspace) ActivateBroadcast(id externalapi.EntityId) {
	w.t.Helper()
	ctx := w.t.Context()
	// Each write names the revision it was made on (ADR 0119): the draft as read, then as the
	// segment change left it.
	read, err := w.api.BroadcastsGet(ctx, externalapi.BroadcastsGetParams{ID: id})
	draft := ok[externalapi.BroadcastResource](w.t, "read broadcast", read, err)
	aud, err := w.api.BroadcastsSetSegment(ctx, &externalapi.SetBroadcastSegmentInput{Revision: draft.Revision, SegmentId: externalapi.NilEntityId{Null: true}},
		externalapi.BroadcastsSetSegmentParams{ID: id})
	targeted := ok[externalapi.BroadcastResource](w.t, "set segment", aud, err)

	sched, err := w.api.BroadcastsSchedule(ctx, &externalapi.ScheduleBroadcastInput{Revision: targeted.Revision, ScheduledAt: externalapi.Timestamp(time.Now())},
		externalapi.BroadcastsScheduleParams{ID: id})
	ok[externalapi.BroadcastResource](w.t, "schedule broadcast", sched, err)
}

// createBroadcast makes the draft through the external API, from the Body or the Doc.
func (w *Workspace) createBroadcast(b Broadcast) externalapi.EntityId {
	w.t.Helper()
	if b.Name == "" {
		b.Name = "e2e broadcast " + uniq()
	}
	email := externalapi.EmailBroadcastContentInput{
		Channel: externalapi.EmailBroadcastContentInputChannelEmail,
		Subject: externalapi.NewOptString(b.Subject),
	}
	if !b.WithoutFrom {
		email.FromName = externalapi.NewOptNilString(w.FromName)
		email.FromEmail = externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(w.FromEmail))
	}
	if len(b.Doc) > 0 {
		email.Doc = externalapi.NewOptEditorDocument(docObject[externalapi.EditorDocument](w.t, b.Doc))
	} else {
		email.Body = externalapi.NewOptString(b.Body)
	}
	in := &externalapi.CreateBroadcastInput{
		Name:    externalapi.Name(b.Name),
		Content: externalapi.BroadcastContentInput{OneOf: externalapi.NewEmailBroadcastContentInputBroadcastContentInputSum(email)},
	}
	res, err := w.api.BroadcastsCreate(w.t.Context(), in)
	return ok[externalapi.BroadcastResource](w.t, "create broadcast", res, err).ID
}

// docObject is a document's JSON as the opaque object the external API takes.
func docObject[M ~map[string]jx.Raw](tb testing.TB, doc json.RawMessage) M {
	tb.Helper()
	var fields map[string]json.RawMessage
	require.NoError(tb, json.Unmarshal(doc, &fields), "the document is a JSON object")
	out := make(M, len(fields))
	for k, v := range fields {
		out[k] = jx.Raw(v)
	}
	return out
}
