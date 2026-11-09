package messaging

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/emersion/go-msgauth/dkim"
	"github.com/wneessen/go-mail"
)

// BuildMIME turns an EmailMessage into an unsigned go-mail *mail.Msg. It is the single
// MIME builder shared by every email provider (smtp and ses send it through
// BuildSignedMessage; the Test inbox keeps it unsigned), so message structure —
// multipart/alternative, headers, encoding — is identical across providers.
//
// From/FromName must already be resolved (msg-level value or provider default)
// by the caller. When both HTML and Text are present the message is
// multipart/alternative with text/plain first and text/html as the richer
// alternative (clients render the last part they understand). A single body is
// sent as-is.
func BuildMIME(msg EmailMessage) (*mail.Msg, error) {
	m := mail.NewMsg()

	if msg.FromName != "" {
		if err := m.FromFormat(msg.FromName, msg.From); err != nil {
			return nil, fmt.Errorf("messaging: invalid from address: %w", err)
		}
	} else if err := m.From(msg.From); err != nil {
		return nil, fmt.Errorf("messaging: invalid from address: %w", err)
	}

	if err := m.To(msg.To); err != nil {
		return nil, fmt.Errorf("messaging: invalid to address: %w", err)
	}
	if msg.ReplyTo != "" {
		if err := m.ReplyTo(msg.ReplyTo); err != nil {
			return nil, fmt.Errorf("messaging: invalid reply-to address: %w", err)
		}
	}
	m.Subject(msg.Subject)
	m.SetMessageID()

	switch {
	case msg.HTML != "" && msg.Text != "":
		m.SetBodyString(mail.TypeTextPlain, msg.Text)
		m.AddAlternativeString(mail.TypeTextHTML, msg.HTML)
	case msg.HTML != "":
		m.SetBodyString(mail.TypeTextHTML, msg.HTML)
	default:
		m.SetBodyString(mail.TypeTextPlain, msg.Text)
	}

	// RFC 8058 one-click unsubscribe (ADR 0012): the HTTPS URI plus the One-Click
	// POST directive. Set only on marketing sends; BuildSignedMessage lists both
	// headers in h= so they ride within the signature.
	if msg.ListUnsubscribeURL != "" {
		m.SetGenHeader(mail.HeaderListUnsubscribe, "<"+msg.ListUnsubscribeURL+">")
		m.SetGenHeader(mail.HeaderListUnsubscribePost, "List-Unsubscribe=One-Click")
	}

	return m, nil
}

// signedHeaders are the header fields the DKIM signature covers on every signed message.
var signedHeaders = []string{"From", "To", "Subject", "Date", "Message-ID", "Content-Type", "MIME-Version"}

// oneClickHeaders are the RFC 8058 one-click unsubscribe headers. RFC 8058 §5 (ADR 0012):
// they must be inside the signed set (h=), or Gmail/Yahoo ignore one-click.
var oneClickHeaders = []string{string(mail.HeaderListUnsubscribe), string(mail.HeaderListUnsubscribePost)}

// Wire is a built message as a provider sends it.
type Wire struct {
	// Bytes is the serialized message, DKIM-signed when it was built with keys.
	Bytes []byte
	// MessageID is the value of its Message-ID header.
	MessageID string
	// Sender and Recipients are its SMTP envelope, the From and To addresses in angle
	// brackets as MAIL FROM and RCPT TO take them.
	Sender     string
	Recipients []string
}

// BuildSignedMessage builds the message via BuildMIME, serializes it and DKIM-signs
// the bytes with the key of msg.From's verified sending domain (ADR 0010). Every
// provider sends the bytes it returns, so a message is signed the same way whatever
// carries it.
//
// The signature uses relaxed canonicalization for header and body, so it survives an
// intermediate MTA refolding a header. A marketing message (ListUnsubscribeURL set)
// also signs its one-click headers.
//
// The gate (slice 3): when keys is non-nil (a workspace send) and From's domain is
// not a verified sending domain, the send is rejected with ErrUnverifiedSendingDomain.
// Nil keys (platform/system mail) are exempt, and the message goes out unsigned.
func BuildSignedMessage(ctx context.Context, msg EmailMessage, keys DKIMKeys) (Wire, error) {
	m, err := BuildMIME(msg)
	if err != nil {
		return Wire{}, err
	}
	sender, err := m.GetSender(false)
	if err != nil {
		return Wire{}, fmt.Errorf("messaging: envelope sender: %w", err)
	}
	recipients, err := m.GetRecipients()
	if err != nil {
		return Wire{}, fmt.Errorf("messaging: envelope recipients: %w", err)
	}
	wire := Wire{MessageID: m.GetMessageID(), Sender: sender, Recipients: recipients}

	var unsigned bytes.Buffer
	if _, err := m.WriteTo(&unsigned); err != nil {
		return Wire{}, fmt.Errorf("messaging: serialize message: %w", err)
	}
	if keys == nil {
		wire.Bytes = unsigned.Bytes()
		return wire, nil
	}
	key, err := keys.Key(ctx, msg.From)
	if err != nil {
		return Wire{}, err
	}
	if key == nil {
		return Wire{}, fmt.Errorf("%w: %s", ErrUnverifiedSendingDomain, msg.From)
	}

	headers := signedHeaders
	if msg.ListUnsubscribeURL != "" {
		headers = slices.Concat(signedHeaders, oneClickHeaders)
	}
	var signed bytes.Buffer
	if err := dkim.Sign(&signed, &unsigned, &dkim.SignOptions{
		Domain:                 key.Domain,
		Selector:               key.Selector,
		Signer:                 key.Signer,
		HeaderCanonicalization: dkim.CanonicalizationRelaxed,
		BodyCanonicalization:   dkim.CanonicalizationRelaxed,
		HeaderKeys:             headers,
	}); err != nil {
		return Wire{}, fmt.Errorf("messaging: dkim sign: %w", err)
	}
	wire.Bytes = signed.Bytes()
	return wire, nil
}
