package outboundmessages

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
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
// Send-eligibility.
type ConfirmationAsker int

const (
	// AskedByWorkspace: a Member, the API or MCP asked. An opt-out of everything refuses the email.
	AskedByWorkspace ConfirmationAsker = iota + 1
	// AskedByPerson: the person asked (an expired link's "send a new link"). Only Suppression stops
	// it.
	AskedByPerson
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
// intro), read when the email is sent. Key is the Confirmation request's own.
type ConfirmationMessage struct {
	Key     string
	Contact *ent.Contact
	AskedBy ConfirmationAsker
	Email   workspaces.ConfirmationEmail
}

// Confirmation is the Request for a confirmation email to the Contact's address. Its content's
// Subject and Body are the Workspace's subject and intro as plain text: the frame around them, the
// Confirm button with its link included, is rendered at send time (renderConfirmation).
func Confirmation(m ConfirmationMessage) Request {
	return Request{
		Key:         m.Key,
		Destination: lo.FromPtr(m.Contact.Email),
		Contact:     m.Contact,
		Content: EmailContent{
			Subject: m.Email.Subject, Body: m.Email.Intro,
			FromEmail: m.Email.FromEmail, FromName: m.Email.FromName,
		},
		Ref: ConfirmationRef{AskedBy: m.AskedBy},
	}
}

// confirmationFrame is the fixed frame of the confirmation email, in the Workspace's Brand kit. The
// kit's values are checked strictly when it is saved (they are inlined into CSS there too), so they
// go in as CSS.
var confirmationFrame = template.Must(template.New("confirmation").Parse(`<!doctype html>
<html><body style="margin:0;padding:0;background-color:{{.Background}}">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:{{.Background}}"><tr><td align="center" style="padding:32px 16px">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;font-family:{{.Font}};color:{{.Text}};font-size:16px;line-height:24px">
{{- if .Logo}}
<tr><td style="padding-bottom:24px"><img src="{{.Logo}}" alt="" height="40" style="display:block;height:40px"></td></tr>
{{- end}}
<tr><td>{{range .Intro}}<p style="margin:0 0 16px">{{.}}</p>{{end}}</td></tr>
<tr><td style="padding:8px 0 24px"><a href="{{.URL}}" style="display:inline-block;padding:12px 24px;background-color:{{.Accent}};color:{{.ButtonText}};border-radius:{{.Radius}}px;text-decoration:none;font-weight:bold">{{.Confirm}}</a></td></tr>
<tr><td style="font-size:13px;line-height:20px;opacity:0.8">{{.NotAsked}}</td></tr>
{{- if .Address}}
<tr><td style="padding-top:24px;font-size:12px;line-height:18px;opacity:0.7">{{range $i, $line := .Address}}{{if $i}}<br>{{end}}{{$line}}{{end}}</td></tr>
{{- end}}
</table>
</td></tr></table>
</body></html>`))

// confirmationView is what the frame is filled with.
type confirmationView struct {
	Background, Font, Text, Accent, ButtonText template.CSS
	Radius                                     int
	Logo                                       string
	Intro                                      []string
	URL                                        string
	Confirm, NotAsked                          string
	Address                                    []string
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
	view := confirmationView{
		Background: template.CSS(kit.BackgroundColor), Font: template.CSS(kit.BodyFont), Text: template.CSS(kit.TextColor),
		Accent: template.CSS(kit.AccentColor), ButtonText: template.CSS(kit.ButtonTextColor), Radius: kit.ButtonRadius,
		Logo:     kit.LogoURL,
		Intro:    paragraphs(content.Body),
		URL:      url,
		Confirm:  i18n.T(i18n.EmailConfirmationRequestConfirm, nil),
		NotAsked: i18n.T(i18n.EmailConfirmationRequestNotAsked, named),
		Address:  paragraphs(ws.PostalAddress),
	}
	var html bytes.Buffer
	if err := confirmationFrame.Execute(&html, view); err != nil {
		return messaging.EmailMessage{}, fmt.Errorf("outbound: render the confirmation email: %w", err)
	}
	return messaging.EmailMessage{
		Subject: content.Subject,
		HTML:    html.String(),
		Text:    emailrender.FinalHTMLToText(html.String()),
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
