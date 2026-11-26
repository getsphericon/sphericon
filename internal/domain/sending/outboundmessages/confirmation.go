package outboundmessages

import (
	"context"
	"fmt"
	stdhtml "html"
	"strings"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/content/brandkits"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/emailrender"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
)

// ConfirmationAsker is who asked for a confirmation email (ADR 0013): it decides the email's mode of
// Send-eligibility. Its values are stored in the queued job of a request, so they never change.
type ConfirmationAsker string

const (
	// AskedByWorkspace: a Member, the API or MCP asked. An opt-out of everything refuses the email.
	AskedByWorkspace ConfirmationAsker = "workspace"
	// AskedByPerson: the person asked (an expired link's "send a new link"). Only Suppression stops
	// it.
	AskedByPerson ConfirmationAsker = "person"
)

// ConfirmationRef is a confirmation email's message: it points at no surface record, its Contact is
// its subject, and who asked for it is its mode.
type ConfirmationRef struct {
	AskedBy ConfirmationAsker
}

func (ConfirmationRef) Kind() outboundmessage.Kind { return outboundmessage.KindConfirmation }

func (ConfirmationRef) stamp(*ent.OutboundMessageScopedCreate) {}

func (ConfirmationRef) owners() (broadcastID, automationID int64) { return 0, 0 }

// mode is the confirmation mode of who asked; an asker no constant names is the zero Mode, which
// Send-eligibility refuses.
func (r ConfirmationRef) mode() eligibility.Mode {
	switch r.AskedBy {
	case AskedByWorkspace:
		return eligibility.ConfirmationByWorkspace()
	case AskedByPerson:
		return eligibility.ConfirmationByPerson()
	}
	return eligibility.Mode{}
}

// ConfirmationMessage is what a confirmation email is made of: the Contact whose address it
// confirms, who asked, and the Workspace's confirmation email as it goes out (its From, subject and
// intro), read when the email is sent. Request is the Confirmation request's own id, which keys its
// message.
type ConfirmationMessage struct {
	Request string
	Contact *ent.Contact
	AskedBy ConfirmationAsker
	Email   workspaces.ConfirmationEmail
}

// Confirmation is the Request for a confirmation email to the Contact's address. Its content's
// Subject and Body are the Workspace's subject and intro as plain text: the frame around them, the
// Confirm button with its link included, is rendered at send time (renderConfirmation).
func Confirmation(m ConfirmationMessage) Request {
	return Request{
		Key:         "confirmation:" + m.Request,
		Destination: lo.FromPtr(m.Contact.Email),
		Contact:     m.Contact,
		Content: EmailContent{
			Subject: m.Email.Subject, Body: m.Email.Intro,
			FromEmail: m.Email.FromEmail, FromName: m.Email.FromName,
		},
		Ref: ConfirmationRef{AskedBy: m.AskedBy},
	}
}

// confirmationFrame is the fixed frame of the confirmation email, in the Workspace's Brand kit: the
// logo, the intro, the Confirm button carrying the link, the line that nothing happens if the person
// did not ask, and the postal address. Every text and attribute is escaped; the kit's values are
// checked strictly when it is saved (they are inlined into CSS there too).
type confirmationFrame struct {
	kit               brandkits.Kit
	intro             []string
	url               string
	confirm, notAsked string
	address           []string
}

// html is the frame filled in.
func (f confirmationFrame) html() string {
	e := stdhtml.EscapeString
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html><body style="margin:0;padding:0;background-color:%s">`, e(f.kit.BackgroundColor))
	fmt.Fprintf(&b, `<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background-color:%s"><tr><td align="center" style="padding:32px 16px">`, e(f.kit.BackgroundColor))
	fmt.Fprintf(&b, `<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;font-family:%s;color:%s;font-size:16px;line-height:24px">`, e(f.kit.BodyFont), e(f.kit.TextColor))
	if f.kit.LogoURL != "" {
		fmt.Fprintf(&b, `<tr><td style="padding-bottom:24px"><img src="%s" alt="" height="40" style="display:block;height:40px"></td></tr>`, e(f.kit.LogoURL))
	}
	b.WriteString(`<tr><td>`)
	for _, p := range f.intro {
		fmt.Fprintf(&b, `<p style="margin:0 0 16px">%s</p>`, e(p))
	}
	b.WriteString(`</td></tr>`)
	fmt.Fprintf(&b, `<tr><td style="padding:8px 0 24px"><a href="%s" style="display:inline-block;padding:12px 24px;background-color:%s;color:%s;border-radius:%dpx;text-decoration:none;font-weight:bold">%s</a></td></tr>`,
		e(f.url), e(f.kit.AccentColor), e(f.kit.ButtonTextColor), f.kit.ButtonRadius, e(f.confirm))
	fmt.Fprintf(&b, `<tr><td style="font-size:13px;line-height:20px;opacity:0.8">%s</td></tr>`, e(f.notAsked))
	if len(f.address) > 0 {
		lines := make([]string, len(f.address))
		for i, line := range f.address {
			lines[i] = e(line)
		}
		fmt.Fprintf(&b, `<tr><td style="padding-top:24px;font-size:12px;line-height:18px;opacity:0.7">%s</td></tr>`, strings.Join(lines, "<br>"))
	}
	b.WriteString(`</table></td></tr></table></body></html>`)
	return b.String()
}

// renderConfirmation renders a confirmation email to dest (ADR 0013): the Workspace's subject and
// intro as text, never Liquid, in the fixed frame, with the Confirm button carrying a link minted now
// (its 7 days run from the send, not from the request, which a Hold may delay), the line that nothing
// happens if the person did not ask, and the postal address when the Workspace has one; in the Brand
// kit as it stands. It carries no unsubscribe link or header, no pixel and no rewritten link.
func (m *Module) renderConfirmation(ctx context.Context, s *ent.Scoped, req Request, dest string, ws *ent.Workspace) (messaging.EmailMessage, error) {
	content, err := emailContentOf(req.Content)
	if err != nil {
		return messaging.EmailMessage{}, err
	}
	if m.tracker == nil {
		return messaging.EmailMessage{}, ErrNoTracker
	}
	tr, err := m.trackerFor(ctx, s)
	if err != nil {
		return messaging.EmailMessage{}, err
	}
	url, err := tr.ConfirmURL(linktarget.Confirmation{Destination: dest, WorkspaceID: ws.ID, ContactID: req.contactID()})
	if err != nil {
		return messaging.EmailMessage{}, fmt.Errorf("outbound: confirmation link: %w", err)
	}
	kit, err := brandkits.NewBrands().Kit(ctx, s)
	if err != nil {
		return messaging.EmailMessage{}, fmt.Errorf("outbound: read the brand kit: %w", err)
	}
	named := map[string]any{"Workspace": ws.Name}
	html := confirmationFrame{
		kit:      kit,
		intro:    paragraphs(content.Body),
		url:      url,
		confirm:  i18n.T(i18n.EmailConfirmationRequestConfirm, nil),
		notAsked: i18n.T(i18n.EmailConfirmationRequestNotAsked, named),
		address:  paragraphs(ws.PostalAddress),
	}.html()
	return messaging.EmailMessage{
		Subject: content.Subject,
		HTML:    html,
		Text:    emailrender.FinalHTMLToText(html),
	}, nil
}

// paragraphs is text split into its non-blank lines, each trimmed.
func paragraphs(text string) []string {
	var out []string
	for line := range strings.Lines(text) {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
