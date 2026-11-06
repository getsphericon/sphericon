// Package secretreencryption is the use case behind `sphericon encryption-key reencrypt`
// (ADR 0102): it stores every secret in the database again under the primary key of the
// Encryption key, so a key an operator rotated away from can be retired. The secrets span every
// Workspace (and the User and Staff member rows no Workspace owns), and each table belongs to a
// domain module: the use case calls the re-encrypt function of each owner, in a fixed order, and
// holds no client itself. Each owner sweeps its columns on the raw client, by id and
// compare-and-swap (secrets.Cipher.Reencrypt). The package also holds the keyset changes of
// `encryption-key rotate` and `retire` and the report of a run.
package secretreencryption

import (
	"context"

	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// Count is what one sweep did to a column.
type Count = secrets.Count

// Owners are the stored secrets of the modules that own them, each built on the raw client by the
// wiring.
type Owners struct {
	Integrations     *integrations.Secrets
	WebhookEndpoints *webhookendpoints.Secrets
	SendingDomains   *sendingdomains.Secrets
	Users            *users.Secrets
	StaffMembers     *staffmembers.Secrets
	Automations      *automations.Secrets
	Rules            *rules.Secrets
}

// UseCase re-encrypts the stored secrets.
type UseCase struct {
	owners Owners
	cipher *secrets.Cipher
}

// New builds the use case on the owners' stored secrets and the cipher whose primary key is the
// target.
func New(owners Owners, cipher *secrets.Cipher) *UseCase {
	return &UseCase{owners: owners, cipher: cipher}
}

// Run sweeps every column that holds ciphertext, owner by owner, and returns what it did per
// column. It fails at the first value the cipher cannot read, which means a key is missing from
// the keyset: nothing guesses, what was already rewritten stays valid, and the counts are those of
// the columns it finished.
func (u *UseCase) Run(ctx context.Context) ([]Count, error) {
	o := u.owners
	owners := []func(context.Context, *secrets.Cipher) ([]secrets.Count, error){
		o.Integrations.Reencrypt,
		o.WebhookEndpoints.Reencrypt,
		o.SendingDomains.Reencrypt,
		o.Users.Reencrypt,
		o.StaffMembers.Reencrypt,
		// Every version, not only the latest: an Enrollment still stands on an old one.
		o.Automations.Reencrypt,
		// Every version, the draft and those of a deleted Rule too: a Firing reads its version.
		o.Rules.Reencrypt,
	}
	var counts []Count
	for _, reencrypt := range owners {
		done, err := reencrypt(ctx, u.cipher)
		counts = append(counts, done...)
		if err != nil {
			return counts, err
		}
	}
	return counts, nil
}
