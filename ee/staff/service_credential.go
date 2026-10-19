package staff

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	entcredential "github.com/getsphericon/sphericon/ent/servicecredential"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
)

// Scope is one thing a Service credential may do on the service endpoints of the
// Staff surface (ADR 0026). A credential carries a set of them and can act on no
// other.
type Scope string

const (
	// ScopeUsageRead reads the Usage of Workspaces.
	ScopeUsageRead Scope = "usage:read"
	// ScopeWorkspacesRead reads the Workspaces and their owners.
	ScopeWorkspacesRead Scope = "workspaces:read"
	// ScopeDomainsRead reads the verified Tracking domain hosts.
	ScopeDomainsRead Scope = "domains:read"
	// ScopeHoldsWrite sets and clears Staff holds.
	ScopeHoldsWrite Scope = "holds:write"
)

// Scopes are every scope a Service credential may carry.
var Scopes = []Scope{ScopeUsageRead, ScopeWorkspacesRead, ScopeDomainsRead, ScopeHoldsWrite}

var (
	// ErrInvalidScope: a scope that no Service credential may carry, or none at all.
	ErrInvalidScope = errors.New("staff: a service credential needs one or more known scopes")
	// ErrInvalidCredential: a Service credential without a name, or with an expiry that
	// is already past.
	ErrInvalidCredential = errors.New("staff: a service credential needs a name and a future expiry")
	// ErrCredentialNotFound: no Service credential has this id.
	ErrCredentialNotFound = errors.New("staff: service credential not found")
	// ErrCredentialInactive: the credential is revoked or expired, so it cannot be rotated.
	ErrCredentialInactive = errors.New("staff: service credential is revoked or expired")
	// ErrForbidden: the credential is valid but its scopes do not cover the operation.
	ErrForbidden = forbiddenError{}
)

// forbiddenError is the 403 of a credential used outside its scopes.
type forbiddenError struct{}

func (forbiddenError) Error() string { return "staff: the credential's scopes do not allow this" }

// Problem is the RFC 7807 answer the composition root renders for the error.
func (forbiddenError) Problem() (status int, code, detail string) {
	return http.StatusForbidden, "", "the credential's scopes do not allow this operation"
}

// lastUsedGranularity is how stale a credential's last-used time may get before a
// request writes it again, so a busy service does not write on every call.
const lastUsedGranularity = time.Minute

// IssueInput is a new Service credential's name, scopes and optional expiry.
type IssueInput struct {
	Name      string
	Scopes    []Scope
	ExpiresAt *time.Time
}

// Issued is a Service credential at the moment it is created: the only time its value
// exists in the clear. Only a hash of it is stored.
type Issued struct {
	ID        int64
	Name      string
	Scopes    []Scope
	ExpiresAt *time.Time
	// Value is what the service presents as its bearer credential. It is shown once.
	Value string
}

// IssueServiceCredential creates a Service credential. A service may hold several at
// once (a rotation needs no downtime), so the name is not unique. ErrNotLicensed
// without the license, ErrInvalidScope and ErrInvalidCredential for a bad input.
func (m *Module) IssueServiceCredential(ctx context.Context, in IssueInput) (*Issued, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || (in.ExpiresAt != nil && !in.ExpiresAt.After(m.now())) {
		return nil, ErrInvalidCredential
	}
	scopes, err := normalizeScopes(in.Scopes)
	if err != nil {
		return nil, err
	}
	prefix, secret, value, err := credentials.GenerateServiceCredential()
	if err != nil {
		return nil, err
	}
	create := m.ent.ServiceCredential.Create().
		SetName(name).
		SetPrefix(prefix).
		SetSecretHash(credentials.HashServiceSecret(secret)).
		SetScopes(scopeStrings(scopes))
	if in.ExpiresAt != nil {
		create.SetExpiresAt(*in.ExpiresAt)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return nil, err
	}
	return &Issued{ID: row.ID, Name: row.Name, Scopes: scopes, ExpiresAt: row.ExpiresAt, Value: value}, nil
}

// RotateServiceCredential issues a new credential with the name and scopes of an
// active one and leaves the old one active, so the service switches over with no
// downtime and the Staff member then revokes the old one. ErrCredentialNotFound for an
// unknown id, ErrCredentialInactive for a revoked or expired one.
func (m *Module) RotateServiceCredential(ctx context.Context, id int64, expiresAt *time.Time) (*Issued, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	old, err := m.ent.ServiceCredential.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, ErrCredentialNotFound
	}
	if err != nil {
		return nil, err
	}
	if !active(old, m.now()) {
		return nil, ErrCredentialInactive
	}
	return m.IssueServiceCredential(ctx, IssueInput{Name: old.Name, Scopes: toScopes(old.Scopes), ExpiresAt: expiresAt})
}

// RevokeServiceCredential ends a credential: it is refused from now on. Revoking a
// revoked credential changes nothing. ErrCredentialNotFound for an unknown id.
func (m *Module) RevokeServiceCredential(ctx context.Context, id int64) error {
	if err := m.licensed(); err != nil {
		return err
	}
	n, err := m.ent.ServiceCredential.Update().
		Where(entcredential.ID(id), entcredential.RevokedAtIsNil()).
		SetRevokedAt(m.now()).
		Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		exists, err := m.ent.ServiceCredential.Query().Where(entcredential.ID(id)).Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return ErrCredentialNotFound
		}
	}
	return nil
}

// ServiceCredentials lists the credentials, oldest first; name narrows them to one
// service's when set. The value of a credential is never recoverable, only its state:
// scopes, expiry, revocation and when it was last used.
func (m *Module) ServiceCredentials(ctx context.Context, name string) ([]*ent.ServiceCredential, error) {
	if err := m.licensed(); err != nil {
		return nil, err
	}
	q := m.ent.ServiceCredential.Query().Order(entcredential.ByID())
	if name = strings.TrimSpace(name); name != "" {
		q = q.Where(entcredential.Name(name))
	}
	return q.All(ctx)
}

// Active reports whether the credential works at time now: neither revoked nor expired.
func (m *Module) Active(c *ent.ServiceCredential) bool { return active(c, m.now()) }

func active(c *ent.ServiceCredential, now time.Time) bool {
	return c.RevokedAt == nil && (c.ExpiresAt == nil || now.Before(*c.ExpiresAt))
}

// AuthenticateService resolves a presented value to its credential and records the use.
// ErrUnauthorized for a value that is malformed, unknown, wrong, revoked or expired (the
// caller cannot tell which), ErrNotLicensed-as-ErrUnauthorized without the license.
func (m *Module) AuthenticateService(ctx context.Context, value string) (*ent.ServiceCredential, error) {
	if err := m.licensed(); err != nil {
		return nil, ErrUnauthorized
	}
	parsed := credentials.ParseServiceCredential(value)
	if parsed == nil {
		return nil, ErrUnauthorized
	}
	row, err := m.ent.ServiceCredential.Query().Where(entcredential.Prefix(parsed.Prefix)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	now := m.now()
	if !credentials.VerifyServiceSecret(parsed.Secret, row.SecretHash) || !active(row, now) {
		return nil, ErrUnauthorized
	}
	if row.LastUsedAt == nil || now.Sub(*row.LastUsedAt) >= lastUsedGranularity {
		if err := m.ent.ServiceCredential.UpdateOneID(row.ID).SetLastUsedAt(now).Exec(ctx); err != nil {
			return nil, err
		}
		row.LastUsedAt = &now
	}
	return row, nil
}

// ServiceActor is who a Service credential's writes are recorded as in the Audit log
// (ADR 0022): kind `service`, named as the Staff member named the credential. A Workspace's
// own team reads it as "Sphericon staff".
func ServiceActor(c *ent.ServiceCredential) events.Actor {
	return events.Actor{Kind: events.ActorService, ID: fmt.Sprint(c.ID), Name: c.Name}
}

func normalizeScopes(in []Scope) ([]Scope, error) {
	if len(in) == 0 {
		return nil, ErrInvalidScope
	}
	out := slices.Clone(in)
	slices.Sort(out)
	out = slices.Compact(out)
	for _, s := range out {
		if !slices.Contains(Scopes, s) {
			return nil, ErrInvalidScope
		}
	}
	return out, nil
}

func scopeStrings(in []Scope) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = string(s)
	}
	return out
}

func toScopes(in []string) []Scope {
	out := make([]Scope, len(in))
	for i, s := range in {
		out[i] = Scope(s)
	}
	return out
}
