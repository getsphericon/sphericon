// Package providers names the sending providers an Integration uses and what a provider reports it
// is doing about an account: the lists an Integration stores and the contract's provider enum
// carries. It is a leaf, so the ent schema types those columns with it.
package providers

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

var allProviders = []Provider{SMTP, SES, Inbox, Telegram, Twilio}

// Values is every Provider as a string, for the ent enum column typed with it.
func (Provider) Values() []string {
	values := make([]string, len(allProviders))
	for i, p := range allProviders {
		values[i] = string(p)
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
