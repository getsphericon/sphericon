package broadcasts

import (
	"context"
	"encoding/json"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/patch"
)

// ContentInput is what a new Broadcast says, in the shape of its Channel (ADR 0078). Its Channel
// is the Broadcast's, fixed from then on (ADR 0080). Each Channel a Broadcast is sent on has its
// own input in this package; EmailContentInput is email's.
type ContentInput interface {
	Channel() messaging.Channel
	// create checks the content and sets it on the new Broadcast q.
	create(ctx context.Context, m *Module, s *ent.Scoped, q *ent.BroadcastScopedCreate) error
}

// ContentPatch is an edit of a draft's content, in the shape of the draft's Channel: a patch of
// another Channel's content is ErrChannelFixed. EmailContentPatch is email's.
type ContentPatch interface {
	Channel() messaging.Channel
	// update checks the edit of the draft cur and sets it on u.
	update(ctx context.Context, m *Module, s *ent.Scoped, cur *ent.Broadcast, u *ent.BroadcastScopedUpdate) error
}

// channelOf is the Broadcast column value of a Channel.
func channelOf(c messaging.Channel) broadcast.Channel { return broadcast.Channel(c) }

// EmailContentInput is the email content of a new Broadcast. A nil field takes its default: an
// empty subject and body, and the Integration's From.
type EmailContentInput struct {
	Subject   *string
	FromName  *string
	FromEmail *string
	Body      *string
	// Doc is the editor's JSON document, empty when the Broadcast is written as HTML. The body is
	// built from it in the same save (ADR 0040).
	Doc json.RawMessage
}

// Channel is email.
func (EmailContentInput) Channel() messaging.Channel { return messaging.ChannelEmail }

// create resolves the body (from the document when one is written), stores it sanitized and
// refuses a From that is not on a verified Sending domain.
func (c EmailContentInput) create(ctx context.Context, m *Module, s *ent.Scoped, q *ent.BroadcastScopedCreate) error {
	resolved, err := emailrender.ResolveContent(ctx, m.renderer, c.Subject, c.Body, c.Doc, ErrBodyAndDoc)
	if err != nil {
		return err
	}
	if setsFrom(c.FromEmail, nil) {
		if err := checkFrom(ctx, s, *c.FromEmail); err != nil {
			return err
		}
	}
	q.SetSubject(lo.FromPtr(c.Subject)).
		SetBody(lo.FromPtr(resolved.Body)).
		SetNillableFromName(c.FromName).
		SetNillableFromEmail(c.FromEmail)
	if resolved.Authored != nil {
		q.SetDoc(c.Doc).SetEditorVersion(resolved.Authored.EditorVersion)
	}
	return nil
}

// EmailContentPatch is an edit of an email draft's content: a field that keeps leaves the stored
// value, a clear drops it. Subject, Body and Doc cannot be cleared; a Body alone clears the stored
// document. FromName and FromEmail clear to the Integration's default.
type EmailContentPatch struct {
	Subject   patch.Field[string]
	FromName  patch.Field[string]
	FromEmail patch.Field[string]
	Body      patch.Field[string]
	Doc       patch.Field[json.RawMessage]
}

// Channel is email.
func (EmailContentPatch) Channel() messaging.Channel { return messaging.ChannelEmail }

// update resolves a written body or document as create does; a From set to a new value must be on
// a verified Sending domain, an unchanged one is not checked again.
func (c EmailContentPatch) update(ctx context.Context, m *Module, s *ent.Scoped, cur *ent.Broadcast, u *ent.BroadcastScopedUpdate) error {
	doc, _ := c.Doc.Get()
	resolved, err := emailrender.ResolveContent(ctx, m.renderer, c.Subject.Ptr(), c.Body.Ptr(), doc, ErrBodyAndDoc)
	if err != nil {
		return err
	}
	if from, ok := c.FromEmail.Get(); ok && setsFrom(&from, cur.FromEmail) {
		if err := checkFrom(ctx, s, from); err != nil {
			return err
		}
	}
	u.SetNillableBody(resolved.Body)
	patch.ApplySet(c.Subject, u.SetSubject)
	patch.Apply(c.FromName, u.SetFromName, u.ClearFromName)
	patch.Apply(c.FromEmail, u.SetFromEmail, u.ClearFromEmail)
	switch {
	case resolved.Authored != nil:
		u.SetDoc(doc).SetEditorVersion(resolved.Authored.EditorVersion)
	case resolved.Body != nil:
		u.ClearDoc().ClearEditorVersion()
	}
	return nil
}
