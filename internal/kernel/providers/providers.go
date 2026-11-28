// Package providers names the sending providers an Integration uses and what a provider reports it
// is doing about an account: the lists an Integration stores and the contract's provider enum
// carries. It is a leaf, so the ent schema types those columns with it.
package providers

import "github.com/getsphericon/sphericon/internal/kernel/channels"

// Provider identifies a concrete provider implementation.
type Provider string

const (
	SMTP Provider = "smtp"
	SES  Provider = "ses"
	// Inbox is the Test inbox: it keeps what it is sent as Captured messages and delivers nothing
	// outside (ADR 0058).
	Inbox Provider = "inbox"
	// Telegram is a Workspace's own Telegram bot, reached through the Bot API.
	Telegram Provider = "telegram"
	// Twilio is a Workspace's own Twilio account, on the SMS Channel.
	Twilio Provider = "twilio"
)

// channelOf is the Channel each Provider sends on, in the order the enum lists them. A Provider
// belongs to exactly one Channel: an Integration's channel is read from here, and the database
// refuses an Integration whose channel is not its provider's (ent/schema/integration.go).
var channelOf = []struct {
	provider Provider
	channel  channels.Channel
}{
	{SMTP, channels.Email},
	{SES, channels.Email},
	{Inbox, channels.Email},
	{Telegram, channels.Telegram},
	{Twilio, channels.SMS},
}

// Channel is the Channel the Provider sends on, "" for a value that names no Provider.
func (p Provider) Channel() channels.Channel {
	for _, c := range channelOf {
		if c.provider == p {
			return c.channel
		}
	}
	return ""
}

// OnChannel is every Provider of the Channel, in enum order.
func OnChannel(ch channels.Channel) []Provider {
	var out []Provider
	for _, c := range channelOf {
		if c.channel == ch {
			out = append(out, c.provider)
		}
	}
	return out
}

// Values is every Provider as a string, for the ent enum column typed with it.
func (Provider) Values() []string {
	values := make([]string, len(channelOf))
	for i, c := range channelOf {
		values[i] = string(c.provider)
	}
	return values
}

// Enforcement is a provider's measure against an account for the mail it sent (SES
// EnforcementStatus).
type Enforcement string

const (
	// EnforcementNone: the account sends normally. An Integration stores it as no value.
	EnforcementNone Enforcement = ""
	// EnforcementProbation: the provider has put the account under review; it still sends.
	EnforcementProbation Enforcement = "probation"
	// EnforcementShutdown: the provider has stopped the account's sending until it is resumed.
	EnforcementShutdown Enforcement = "shutdown"
)

// Values is every measure an Integration stores (EnforcementNone is stored as no value), for the
// ent enum column typed with it.
func (Enforcement) Values() []string {
	return []string{string(EnforcementProbation), string(EnforcementShutdown)}
}

// KeepsEnvelope reports whether the Provider keeps a Sending domain's envelope (its MAIL FROM) as a
// setting of its own the app can read (SES's custom MAIL FROM, ADR 0184). An SMTP relay's envelope
// is set at a provider the app does not know; the Test inbox sends nowhere.
func (p Provider) KeepsEnvelope() bool { return p == SES }

// MailFromStatus is what a provider last said of a custom MAIL FROM domain it was told to use for a
// Sending domain (SES MailFromDomainStatus, ADR 0184): whether it found the domain's MX record.
type MailFromStatus string

const (
	// MailFromPending: the provider has not looked for the MX record yet.
	MailFromPending MailFromStatus = "pending"
	// MailFromSuccess: the provider found the MX record and sends with the custom domain.
	MailFromSuccess MailFromStatus = "success"
	// MailFromFailed: the provider cannot find the MX record, or it no longer exists.
	MailFromFailed MailFromStatus = "failed"
	// MailFromTemporaryFailure: a passing issue kept the provider from telling.
	MailFromTemporaryFailure MailFromStatus = "temporary_failure"
)

// Values is every status, for the ent enum column typed with it.
func (MailFromStatus) Values() []string {
	return []string{string(MailFromPending), string(MailFromSuccess), string(MailFromFailed), string(MailFromTemporaryFailure)}
}

// MXFailureBehavior is what a provider does with a send while the custom MAIL FROM domain's MX
// record fails (SES BehaviorOnMxFailure).
type MXFailureBehavior string

const (
	// MXFailureFallBack: the provider sends with its own MAIL FROM domain instead.
	MXFailureFallBack MXFailureBehavior = "fall_back"
	// MXFailureReject: the provider refuses the send.
	MXFailureReject MXFailureBehavior = "reject"
)

// Values is every behaviour, for the ent enum column typed with it.
func (MXFailureBehavior) Values() []string {
	return []string{string(MXFailureFallBack), string(MXFailureReject)}
}
