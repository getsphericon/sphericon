package messaging

import "context"

// ConfigurationSetChecker is implemented by a sender whose provider can tell whether the
// configuration set its sends name exists (SES). Saving an Integration asks it, best effort: a
// missing set is a warning, never a refusal.
type ConfigurationSetChecker interface {
	// ConfigurationSetMissing reports whether the provider says it has no such set; false when the
	// sender names none. An error is no answer (no permission, a compatible service without the
	// call, a timeout).
	ConfigurationSetMissing(ctx context.Context) (bool, error)
}

// AsConfigurationSetChecker reports the ConfigurationSetChecker behind sender, looking through
// the metrics decorator Catalog.BuildEmail wraps every sender in, as AsQuotaReader does.
func AsConfigurationSetChecker(sender EmailSender) (ConfigurationSetChecker, bool) {
	if i, ok := sender.(instrumentedSender); ok {
		sender = i.EmailSender
	}
	checker, ok := sender.(ConfigurationSetChecker)
	return checker, ok
}
