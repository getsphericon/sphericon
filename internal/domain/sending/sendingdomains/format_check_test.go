package sendingdomains_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The database refuses a domain a write did not normalise, whatever code writes it: a second
// spelling of one domain would escape the unique index (sending_domains_domain_format; the Join and
// Tracking domains have the same check).
func TestTheDatabaseRefusesAnUnnormalisedDomain(t *testing.T) {
	for _, domain := range []string{"Mail.Example.com", "mail.example.com.", "bücher.example"} {
		t.Run(domain, func(t *testing.T) {
			env := testhelper.Setup(t)
			// test writes: the refused insert is the operation under test.
			err := env.DB.Scoped(fixtures.AcmeID).SendingDomain().Create().
				SetDomain(domain).SetDkimSelector("s1").SetDkimPrivateKeyEncrypted("k").SetDkimPublicKey("p").
				Exec(t.Context())
			require.Error(t, err)
			assert.True(t, ent.IsConstraintError(err), "a constraint refuses it: %v", err)
		})
	}
}
