package integrations

import (
	"context"
	"errors"
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

// ErrKeepsNoEnvelope is an Integration whose provider keeps no envelope to read
// (providers.Provider.KeepsEnvelope): ReadMailFrom asks it nothing.
var ErrKeepsNoEnvelope = errors.New("integrations: the provider keeps no envelope")

// ReadMailFrom reads the custom MAIL FROM of domain's identity at the Integration's provider with
// its credentials, bounded as a quota lookup is (ADR 0184). An Integration whose provider keeps no
// envelope is ErrKeepsNoEnvelope; any other error is no answer: a stored config that cannot be
// read, no permission, no identity of the domain, a timeout.
func (m *Module) ReadMailFrom(ctx context.Context, integ *ent.Integration, domain string) (messaging.MailFrom, error) {
	if !integ.Provider.KeepsEnvelope() {
		return messaging.MailFrom{}, ErrKeepsNoEnvelope
	}
	config, err := m.cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return messaging.MailFrom{}, fmt.Errorf("decrypt integration %d config: %w", integ.ID, err)
	}
	built, err := m.catalog.BuildEmail(integ.Provider, config, messaging.Deps{})
	if err != nil {
		return messaging.MailFrom{}, err
	}
	reader, ok := messaging.AsMailFromReader(built)
	if !ok {
		return messaging.MailFrom{}, fmt.Errorf("integration %d: its %s sender reads no MAIL FROM", integ.ID, integ.Provider)
	}
	ctx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	return reader.MailFrom(ctx, domain)
}
