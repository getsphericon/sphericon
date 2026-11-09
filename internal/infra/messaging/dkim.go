package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// ErrUnverifiedSendingDomain is returned by the send path when a workspace
// message's From domain is not a verified sending domain (ADR 0010 slice 3): a
// verified domain is required to send. Callers translate it — RFC 7807 on the
// sync transactional API, a failed recipient/run on the async broadcast and
// automation surfaces.
var ErrUnverifiedSendingDomain = errors.New("sender domain is not a verified sending domain")

// dkimKeys is the workspace-bound DKIMKeys: it maps an outbound From address to
// the workspace's verified sending domain and opens the domain's Tink-encrypted
// private key (ADR 0010). Bound to one workspace so every lookup is workspace-scoped.
type dkimKeys struct {
	scope  *ent.Scoped
	cipher *secrets.Cipher
}

// NewDKIMKeys builds the DKIMKeys of the Workspace the scoped client is confined to.
func NewDKIMKeys(s *ent.Scoped, cipher *secrets.Cipher) DKIMKeys {
	return &dkimKeys{scope: s, cipher: cipher}
}

// Key returns the key of fromEmail's domain when that domain is a verified sending
// domain in this workspace, or (nil, nil) otherwise; BuildSignedMessage turns that
// into ErrUnverifiedSendingDomain. The match is on the exact From domain; a
// subdomain of a verified domain does not match.
func (k *dkimKeys) Key(ctx context.Context, fromEmail string) (*DKIMKey, error) {
	domain := DomainOf(fromEmail)
	if domain == "" {
		return nil, nil
	}

	row, err := k.scope.SendingDomain().Query().
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

	privPEM, err := k.cipher.Decrypt(row.DkimPrivateKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("messaging: decrypt dkim key for %s: %w", domain, err)
	}
	key, err := mail.PrivKeyFromPEM(privPEM)
	if err != nil {
		return nil, fmt.Errorf("messaging: parse dkim key for %s: %w", domain, err)
	}
	return &DKIMKey{Domain: row.Domain, Selector: row.DkimSelector, Signer: key}, nil
}

// HasVerifiedSendingDomain reports whether fromEmail's domain is a verified
// sending domain in the workspace. It is the plan-time counterpart to the
// send-time gate in BuildSignedMessage, letting callers (e.g. broadcast planning)
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

// DomainOf returns the lower-cased domain of an email address, or "" when there
// is no single "@".
func DomainOf(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}
