package messaging

import (
	"context"

	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// MailFrom is what an email provider says of the envelope domain (MAIL FROM, Return-Path) it sends
// a Sending domain's mail with, where SPF is checked (ADR 0184). The zero value is the provider's
// own domain: no custom one is set.
type MailFrom struct {
	// Domain is the custom MAIL FROM domain set at the provider; empty is none.
	Domain string
	// Status is the provider's status of Domain; empty when Domain is.
	Status providers.MailFromStatus
	// OnMXFailure is what the provider does with a send while Domain's MX record fails; empty when
	// Domain is.
	OnMXFailure providers.MXFailureBehavior
}

// MailFromReader is implemented by a sender whose provider keeps a Sending domain's envelope as a
// setting of its own (SES's custom MAIL FROM). It only reads: the setting is the sender's to make
// at the provider.
type MailFromReader interface {
	// MailFrom reads the envelope the provider uses for domain. An error is no answer: no
	// permission, no identity of the domain at the provider, an answer it cannot read, a timeout.
	MailFrom(ctx context.Context, domain string) (MailFrom, error)
}

// AsMailFromReader reports the MailFromReader behind sender, looking through the metrics decorator
// Catalog.BuildEmail wraps every sender in, as AsConfigurationSetChecker does.
func AsMailFromReader(sender EmailSender) (MailFromReader, bool) {
	if i, ok := sender.(instrumentedSender); ok {
		sender = i.EmailSender
	}
	reader, ok := sender.(MailFromReader)
	return reader, ok
}
