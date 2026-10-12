package messaging

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// ErrNoProvider is returned when a workspace has no enabled default provider for
// the requested channel.
var ErrNoProvider = fmt.Errorf("no default provider configured")

// Resolver turns a workspace's stored integration into a live sender. It is the
// only in-scope consumer of the catalog's Build path; the marketing send engine
// that actually dispatches campaigns is a separate, future component.
type Resolver struct {
	cipher  *secrets.Cipher
	catalog *Catalog
}

// NewResolver wires the resolver.
func NewResolver(cipher *secrets.Cipher, catalog *Catalog) *Resolver {
	return &Resolver{cipher: cipher, catalog: catalog}
}

// EmailSender resolves the workspace's default, enabled email provider, decrypts
// its config and builds a ready sender. Returns ErrNoProvider when none exists.
func (r *Resolver) EmailSender(ctx context.Context, s *ent.Scoped) (EmailSender, error) {
	row, err := DefaultEmailIntegration(ctx, s)
	if err != nil {
		return nil, err
	}

	config, err := r.cipher.Decrypt(row.ConfigEncrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt integration %d config: %w", row.ID, err)
	}
	signer := NewDKIMSigner(s, r.cipher)
	return r.catalog.BuildEmail(Provider(row.Provider), config, signer)
}

// DefaultEmailIntegration is the Workspace's default, enabled email Integration: the
// one Outbound send goes through. It returns ErrNoProvider when there is none.
func DefaultEmailIntegration(ctx context.Context, s *ent.Scoped) (*ent.Integration, error) {
	row, err := s.Integration().Query().
		Where(
			integration.ChannelEQ(integration.ChannelEmail),
			integration.IsDefault(true),
			integration.Enabled(true),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNoProvider
	}
	return row, err
}
