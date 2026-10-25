// Package messaging is the channel-agnostic management layer for workspace
// sending providers. It defines the per-channel send contracts (EmailSender
// today; SmsSender later), a provider catalog that maps a (channel, provider)
// pair to its validation + construction logic, and a resolver that turns a
// workspace's stored integration into a ready-to-use sender.
//
// The split is deliberate: storage, encryption, CRUD, default-selection and the
// catalog are generic across channels, while the send interface is channel
// specific. Adding a provider = register a descriptor + an impl. Adding a
// channel = a new send interface + descriptors, reusing all of the above.
package messaging

import (
	"context"

	"github.com/wneessen/go-mail"
)

// Channel identifies a delivery medium. Values mirror the ent Integration.channel enum.
type Channel string

const (
	ChannelEmail Channel = "email"
	// ChannelSMS is reserved; no SMS provider is implemented yet.
	ChannelSMS Channel = "sms"
)

// Provider identifies a concrete provider implementation. Values mirror the ent
// Integration.provider enum.
type Provider string

const (
	ProviderSMTP Provider = "smtp"
	ProviderSES  Provider = "ses"
)

// EmailMessage is the channel-specific payload for email sends. HTML is used
// when set, otherwise Text (the provider libraries take a single body).
type EmailMessage struct {
	From     string
	FromName string
	To       string
	Subject  string
	HTML     string
	Text     string
	// ListUnsubscribeURL, when set, emits the RFC 8058 one-click unsubscribe
	// headers (List-Unsubscribe + List-Unsubscribe-Post). Set only on marketing
	// surfaces (Broadcast, Automation); never on Transactional (ADR 0012).
	ListUnsubscribeURL string
}

// EmailSender is implemented by every email provider (smtp, ses, …).
type EmailSender interface {
	Send(ctx context.Context, msg EmailMessage) (Receipt, error)
}

// Receipt is what a provider returns for a message it accepted.
type Receipt struct {
	// MessageID is the provider's id for the accepted message (SES MessageId, or the
	// MIME Message-ID for SMTP). Recorded on the Outbound message for correlation.
	MessageID string
	// DeliveryConfirmed is set by a provider that confirmed delivery when it accepted the
	// message (a Test inbox keeps it, so nothing later reports it). Recording the send then
	// stamps the delivery result. SMTP and SES never set it: their Delivery notices arrive
	// later from the provider.
	DeliveryConfirmed bool
}

// DefaultFromer is implemented by senders that fall back to the integration's
// configured From when a message carries none. Outbound send reads it to know the
// effective From domain before sending, so the verified-domain gate and the
// Sending-domain stamp use the address that will really go on the wire.
type DefaultFromer interface {
	DefaultFrom() (addr, name string)
}

// OutsideDeliverer is implemented by a sender that says whether its provider delivers
// outside sphericon. A sender that does not implement it delivers outside (SMTP, SES).
// Outbound send reads it through DeliversOutside, never by the provider's name: the
// Sending-domain gate and DKIM signing apply only to a sender that delivers outside.
type OutsideDeliverer interface {
	DeliversOutside() bool
}

// DeliversOutside reports whether sender's provider delivers outside, looking through the
// metrics decorator Catalog.BuildEmail wraps every sender in. A sender that says nothing
// delivers outside.
func DeliversOutside(sender EmailSender) bool {
	if i, ok := sender.(instrumentedSender); ok {
		sender = i.EmailSender
	}
	if d, ok := sender.(OutsideDeliverer); ok {
		return d.DeliversOutside()
	}
	return true
}

// Signer resolves the native DKIM signer for an outbound message (ADR 0010).
// Signing is sphericon-native and transport-independent: the same signer applies
// identically across providers because both serialize the message via go-mail's
// WriteTo, which signs when a DKIM signer is set.
type Signer interface {
	// DKIMSigner returns a go-mail DKIM signer for mail sent from fromEmail, or
	// (nil, nil) when fromEmail's domain has no verified sending domain in scope
	// (slice 2 signs when possible; the send gate for unverified From is slice 3).
	DKIMSigner(ctx context.Context, fromEmail string) (*mail.DKIMSigner, error)
}

// FormatSender renders an email sender as "Name <addr>" when a display name is
// present, otherwise just the address.
func FormatSender(addr, name string) string {
	if name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}
