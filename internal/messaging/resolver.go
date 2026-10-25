package messaging

import (
	"context"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// ErrNoProvider is returned when a workspace has no enabled default provider for
// the requested channel.
var ErrNoProvider = errors.New("no default provider configured")

// Resolver turns a workspace's stored integration into a live sender. It is the
// only in-scope consumer of the catalog's Build path; the marketing send engine
// that actually dispatches campaigns is a separate, future component.
type Resolver struct {
	cipher    *secrets.Cipher
	catalog   *Catalog
	capturers func(s *ent.Scoped, integrationID int64) Capturer
}

// ResolverOption configures a Resolver.
type ResolverOption func(*Resolver)

// WithCapturers binds the Capturer a provider builder receives to the Workspace's scoped
// handle and to the Integration the sender is built for.
func WithCapturers(bind func(s *ent.Scoped, integrationID int64) Capturer) ResolverOption {
	return func(r *Resolver) { r.capturers = bind }
}

// NewResolver wires the resolver.
func NewResolver(cipher *secrets.Cipher, catalog *Catalog, opts ...ResolverOption) *Resolver {
	r := &Resolver{cipher: cipher, catalog: catalog}
	for _, o := range opts {
		o(r)
	}
	return r
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
	deps := Deps{Signer: NewDKIMSigner(s, r.cipher)}
	if r.capturers != nil {
		deps.Capturer = r.capturers(s, row.ID)
	}
	return r.catalog.BuildEmail(Provider(row.Provider), config, deps)
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
