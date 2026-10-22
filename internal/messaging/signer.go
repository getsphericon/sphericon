package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// ErrUnverifiedSendingDomain is returned by the send path when a workspace
// message's From domain is not a verified sending domain (ADR 0010 slice 3): a
// verified domain is required to send. Callers translate it — RFC 7807 on the
// sync transactional API, a failed recipient/run on the async broadcast and
// automation surfaces.
var ErrUnverifiedSendingDomain = errors.New("sender domain is not a verified sending domain")

// dkimSigner is the workspace-bound Signer: it maps an outbound From address to
// the workspace's verified sending domain and builds a native go-mail DKIM
// signer from the domain's Tink-encrypted private key (ADR 0010). Bound to one
// workspace so every lookup is workspace-scoped.
type dkimSigner struct {
	scope  *ent.Scoped
	cipher *secrets.Cipher
}

// NewDKIMSigner builds a Signer for the Workspace the scoped client is confined to.
func NewDKIMSigner(s *ent.Scoped, cipher *secrets.Cipher) Signer {
	return &dkimSigner{scope: s, cipher: cipher}
}

// DKIMSigner returns a signer for fromEmail's domain when that domain is a
// verified sending domain in this workspace, or (nil, nil) otherwise — an
// unverified or unknown From is left unsigned here (the send gate is slice 3).
// The match is on the exact From domain; a subdomain of a verified domain does
// not match and is left unsigned.
func (s *dkimSigner) DKIMSigner(ctx context.Context, fromEmail string) (*mail.DKIMSigner, error) {
	domain := DomainOf(fromEmail)
	if domain == "" {
		return nil, nil
	}

	row, err := s.scope.SendingDomain().Query().
		Where(
			sendingdomain.Domain(domain),
			dkimstate.Verified(),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	privPEM, err := s.cipher.Decrypt(row.DkimPrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("messaging: decrypt dkim key for %s: %w", domain, err)
	}
	key, err := mail.PrivKeyFromPEM(privPEM)
	if err != nil {
		return nil, fmt.Errorf("messaging: parse dkim key for %s: %w", domain, err)
	}
	// Default signer signs the standard header set with relaxed canonicalization.
	// The one-click List-Unsubscribe headers are added to the signed set per-message
	// in BuildSignedMIME (only marketing sends carry them) — see the note there.
	return mail.NewDKIMSigner(row.Domain, row.DkimSelector, key), nil
}

// HasVerifiedSendingDomain reports whether fromEmail's domain is a verified
// sending domain in the workspace. It is the plan-time counterpart to the
// send-time gate in BuildSignedMIME, letting callers (e.g. broadcast planning)
// reject up front instead of failing every recipient. An empty/malformed
// fromEmail reports false.
func HasVerifiedSendingDomain(ctx context.Context, s *ent.Scoped, fromEmail string) (bool, error) {
	domain := DomainOf(fromEmail)
	if domain == "" {
		return false, nil
	}
	return s.SendingDomain().Query().
		Where(
			sendingdomain.Domain(domain),
			dkimstate.Verified(),
		).
		Exist(ctx)
}

// domainOf returns the lower-cased domain of an email address, or "" when there
// is no single "@".
func DomainOf(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}
