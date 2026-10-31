package messaging

import "context"

// Quota is what a provider reports about the customer's account: its sending limits (ADR
// 0023) and its standing (ADR 0067). A nil limit is a window the provider does not bound;
// the zero value of the rest is an account with nothing to flag.
type Quota struct {
	PerSecond *int
	PerDay    *int
	// Enforcement is what the provider is doing about the account's sending.
	Enforcement Enforcement
	// Sandbox is set while the provider delivers only to the account's verified addresses.
	Sandbox bool
}

// Enforcement is a provider's measure against an account for the mail it sent (SES
// EnforcementStatus).
type Enforcement string

const (
	// EnforcementNone: the account sends normally.
	EnforcementNone Enforcement = ""
	// EnforcementProbation: the provider has put the account under review; it still sends.
	EnforcementProbation Enforcement = "probation"
	// EnforcementShutdown: the provider has stopped the account's sending until it is resumed.
	EnforcementShutdown Enforcement = "shutdown"
)

// QuotaReader is implemented by a sender whose provider can report its quota (SES
// GetAccount). Providers that cannot (SMTP) do not implement it and have no
// provider ceiling.
type QuotaReader interface {
	SendQuota(ctx context.Context) (Quota, error)
}

// AsQuotaReader reports the QuotaReader behind sender, looking through the metrics
// decorator Catalog.BuildEmail wraps every sender in (a type assertion on the
// decorator alone would never see the provider's own interface).
func AsQuotaReader(sender EmailSender) (QuotaReader, bool) {
	if i, ok := sender.(instrumentedSender); ok {
		sender = i.EmailSender
	}
	reader, ok := sender.(QuotaReader)
	return reader, ok
}
