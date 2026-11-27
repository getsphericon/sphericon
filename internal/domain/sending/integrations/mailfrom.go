package integrations

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// EnvelopeIntegrations returns the Workspace's kept email Integrations whose envelope a Sending
// domain shows (ADR 0184), ascending by id: every one but the Test inbox, which sends nowhere.
func (m *Module) EnvelopeIntegrations(ctx context.Context, s *ent.Scoped) ([]*ent.Integration, error) {
	return s.Integration().Query().
		Where(integration.ChannelEQ(channels.Email), integration.ProviderNEQ(providers.Inbox)).
		Order(ent.Asc(integration.FieldID)).
		All(ctx)
}

// ReadMailFrom reads the custom MAIL FROM of domain's identity at the Integration's provider with
// its credentials, bounded as a quota lookup is (ADR 0184); keeps is false for a provider that
// keeps no such setting (SMTP), which is asked nothing. An error is no answer: a stored config
// that cannot be read, no permission, no identity of the domain, a timeout.
func (m *Module) ReadMailFrom(ctx context.Context, integ *ent.Integration, domain string) (mailFrom messaging.MailFrom, keeps bool, err error) {
	if integ.Channel != channels.Email {
		return messaging.MailFrom{}, false, nil
	}
	config, err := m.cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return messaging.MailFrom{}, true, fmt.Errorf("decrypt integration %d config: %w", integ.ID, err)
	}
	built, err := m.catalog.BuildEmail(integ.Provider, config, messaging.Deps{})
	if err != nil {
		return messaging.MailFrom{}, true, err
	}
	reader, ok := messaging.AsMailFromReader(built)
	if !ok {
		return messaging.MailFrom{}, false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	mailFrom, err = reader.MailFrom(ctx, domain)
	return mailFrom, true, err
}
