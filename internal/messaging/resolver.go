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

// EmailSender decrypts the config of row, an Integration the caller resolved with
// EmailIntegration, and builds a ready sender. Returns ErrNoProvider when row is nil.
func (r *Resolver) EmailSender(_ context.Context, s *ent.Scoped, row *ent.Integration) (EmailSender, error) {
	if row == nil {
		return nil, ErrNoProvider
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
	return EmailIntegration(ctx, s, nil)
}

// IntegrationKept reports whether the Workspace has a kept Integration with the id: new work
// may name it. An archived one leaves new work (ADR 0049), though work already naming it keeps
// it as history, and one of another Workspace is not there. It says nothing of whether the
// Integration can send: the Send route of outbound decides that, for a send and for the moments
// that arm one (an Automation's Publish, a Broadcast's Schedule).
func IntegrationKept(ctx context.Context, s *ent.Scoped, id int64) (bool, error) {
	return s.Integration().Query().Where(integration.ID(id)).Exist(ctx)
}

// EmailIntegration is the enabled email Integration a send goes through: the one named by
// id, or the Workspace's default when id is nil. A named Integration that is disabled or
// discarded is ErrNoProvider, never the default in its place. A send asks the Send route of
// outbound (outbound.Module.Route), which reads it, never this directly.
func EmailIntegration(ctx context.Context, s *ent.Scoped, id *int64) (*ent.Integration, error) {
	which := integration.IsDefault(true)
	if id != nil {
		which = integration.ID(*id)
	}
	row, err := s.Integration().Query().
		Where(
			integration.ChannelEQ(integration.ChannelEmail),
			which,
			integration.Enabled(true),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrNoProvider
	}
	return row, err
}
