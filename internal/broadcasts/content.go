package broadcasts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/smsrender"
	"github.com/getsphericon/sphericon/internal/telegramrender"
)

// ContentInput is what a new Broadcast says, in the shape of its Channel (ADR 0078). Its Channel
// is the Broadcast's, fixed from then on (ADR 0080). Each Channel a Broadcast is sent on has its
// own input in this package: EmailContentInput is email's, TelegramContentInput Telegram's,
// SMSContentInput SMS's.
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

// TelegramContentInput is the Telegram content of a new Broadcast, the same Message content as a
// Telegram step (ADR 0078). A field the write leaves out takes its default: an empty text, no image,
// no buttons, the link preview on, no unsubscribe button.
type TelegramContentInput struct {
	Text              *string
	ImageURL          *string
	Buttons           *[]telegramrender.Button
	LinkPreview       *bool
	UnsubscribeButton *string
}

// Channel is Telegram.
func (TelegramContentInput) Channel() messaging.Channel { return messaging.ChannelTelegram }

// create stores the content. A text whose Liquid does not render, an image that is no https URL,
// too many buttons or a button without a label or an http(s) URL are refused; the text's length is
// checked at Schedule, not here, so a draft can be written past the limit and cut down.
func (c TelegramContentInput) create(_ context.Context, _ *Module, _ *ent.Scoped, q *ent.BroadcastScopedCreate) error {
	if err := checkTelegram(c.Text, c.ImageURL, c.Buttons, c.UnsubscribeButton); err != nil {
		return err
	}
	q.SetText(lo.FromPtr(c.Text))
	if image := strings.TrimSpace(lo.FromPtr(c.ImageURL)); image != "" {
		q.SetImageURL(image)
	}
	q.SetButtons(lo.FromPtr(c.Buttons))
	q.SetLinkPreview(lo.FromPtrOr(c.LinkPreview, true))
	if label := strings.TrimSpace(lo.FromPtr(c.UnsubscribeButton)); label != "" {
		q.SetUnsubscribeButton(label)
	}
	return nil
}

// TelegramContentPatch is an edit of a Telegram draft's content: a field that keeps leaves the
// stored one. The text cannot be cleared; a blank image or unsubscribe label removes it, an empty
// list of buttons removes them.
type TelegramContentPatch struct {
	Text              patch.Field[string]
	ImageURL          patch.Field[string]
	Buttons           patch.Field[[]telegramrender.Button]
	LinkPreview       patch.Field[bool]
	UnsubscribeButton patch.Field[string]
}

// Channel is Telegram.
func (TelegramContentPatch) Channel() messaging.Channel { return messaging.ChannelTelegram }

func (c TelegramContentPatch) update(_ context.Context, _ *Module, _ *ent.Scoped, _ *ent.Broadcast, u *ent.BroadcastScopedUpdate) error {
	if err := checkTelegram(c.Text.Ptr(), c.ImageURL.Ptr(), c.Buttons.Ptr(), c.UnsubscribeButton.Ptr()); err != nil {
		return err
	}
	patch.ApplySet(c.Text, u.SetText)
	patch.Apply(blankToClear(c.ImageURL), func(v string) *ent.BroadcastScopedUpdate { return u.SetImageURL(strings.TrimSpace(v)) }, u.ClearImageURL)
	patch.ApplySet(c.Buttons, u.SetButtons)
	patch.ApplySet(c.LinkPreview, u.SetLinkPreview)
	patch.Apply(blankToClear(c.UnsubscribeButton), func(v string) *ent.BroadcastScopedUpdate { return u.SetUnsubscribeButton(strings.TrimSpace(v)) }, u.ClearUnsubscribeButton)
	return nil
}

// SMSContentInput is the SMS content of a new Broadcast, the same Message content as an SMS step
// (ADR 0078): one plain text with Liquid merge tags, and whether its links go out as short tracked
// links (ADR 0083). A field the write leaves out takes its default: an empty text, link tracking on.
type SMSContentInput struct {
	Text         *string
	LinkTracking *bool
}

// Channel is SMS.
func (SMSContentInput) Channel() messaging.Channel { return messaging.ChannelSMS }

// create stores the content. A text whose Liquid does not render, or longer than the provider sends, is
// refused; a blank one is refused at Schedule, not here, so a draft can be saved before it is
// written.
func (c SMSContentInput) create(ctx context.Context, m *Module, s *ent.Scoped, q *ent.BroadcastScopedCreate) error {
	tracked := lo.FromPtrOr(c.LinkTracking, true)
	if c.Text != nil {
		if err := m.checkSMS(ctx, s, *c.Text, tracked); err != nil {
			return err
		}
	}
	q.SetText(lo.FromPtr(c.Text)).SetLinkTracking(tracked)
	return nil
}

// SMSContentPatch is an edit of an SMS draft's content: a field that keeps leaves the stored one;
// the text cannot be cleared.
type SMSContentPatch struct {
	Text         patch.Field[string]
	LinkTracking patch.Field[bool]
}

// Channel is SMS.
func (SMSContentPatch) Channel() messaging.Channel { return messaging.ChannelSMS }

// update checks the text the draft will hold, the stored one when only link tracking changes:
// turning tracking off counts its links as written, which can take it over the provider's length limit.
func (c SMSContentPatch) update(ctx context.Context, m *Module, s *ent.Scoped, cur *ent.Broadcast, u *ent.BroadcastScopedUpdate) error {
	text, textSet := c.Text.Get()
	tracked, trackingSet := c.LinkTracking.Get()
	if textSet || trackingSet {
		if !textSet {
			text = lo.FromPtr(cur.Text)
		}
		if !trackingSet {
			tracked = cur.LinkTracking
		}
		if err := m.checkSMS(ctx, s, text, tracked); err != nil {
			return err
		}
	}
	patch.ApplySet(c.Text, u.SetText)
	patch.ApplySet(c.LinkTracking, u.SetLinkTracking)
	return nil
}

// checkSMS refuses a text whose Liquid does not render, as email's content is refused, and one
// longer than the provider sends, as an SMS step's save does: its links
// counted as the Workspace's short links when tracked, as written when not. Twilio is the one SMS
// provider, so its limit is the one a text is held to.
func (m *Module) checkSMS(ctx context.Context, s *ent.Scoped, text string, tracked bool) error {
	linkLength := 0
	if tracked {
		var err error
		if linkLength, err = m.out.ShortLinkLength(ctx, s); err != nil {
			return err
		}
	}
	if err := errors.Join(smsrender.Check(text), smsrender.CheckLength(text, twilio.MaxLength, linkLength)); err != nil {
		return fmt.Errorf("%w: %w", emailrender.ErrInvalidContent, err)
	}
	return nil
}

// blankToClear turns a write of a blank text into a clear: an optional text column holds no blank.
func blankToClear(f patch.Field[string]) patch.Field[string] {
	if v, ok := f.Get(); ok && strings.TrimSpace(v) == "" {
		return patch.Clear[string]()
	}
	return f
}

// checkTelegram refuses the parts of a Telegram message that are given and that sphericon or
// Telegram would refuse, as email's content is refused: a text whose Liquid does not render, an
// image that is no https URL, too many or malformed buttons, a label too long.
func checkTelegram(text, image *string, buttons *[]telegramrender.Button, unsubscribe *string) error {
	var errs []error
	if text != nil {
		errs = append(errs, telegramrender.Check(*text))
	}
	if image != nil {
		errs = append(errs, telegramrender.CheckImage(*image))
	}
	if buttons != nil {
		errs = append(errs, telegramrender.CheckButtons(*buttons))
	}
	if unsubscribe != nil {
		errs = append(errs, telegramrender.CheckUnsubscribeButton(*unsubscribe))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%w: %w", emailrender.ErrInvalidContent, err)
	}
	return nil
}
