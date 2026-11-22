package testhelper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/insights/usages"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	entusage "github.com/getsphericon/sphericon/ent/usage"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A test's own write of a row the hooks watch goes through Feed, which makes it as production makes
// every such write: in a transaction of the bus, whose hooks record it and whose work before the
// commit runs. The contacts peak, which the Usage hook raises right before the commit, counts the
// Contact Feed added, on a rolled-back env and on a committed one alike.
func TestAFeedWriteIsRecordedAsABusTransactionRecordsIt(t *testing.T) {
	envs := map[string][]testhelper.Option{"rolled back": nil, "committed": {testhelper.Committed()}}
	for name, opts := range envs {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t, opts...)
			s := env.DB.Scoped(fixtures.AcmeID)
			base, err := s.Contact().Query().Where(contact.EmailNotNil()).Count(t.Context())
			require.NoError(t, err)

			env.Feed(t, func(tx *ent.Client) error {
				// test writes: the contacts-peak hook fires on this Contact write
				return tx.Scoped(fixtures.AcmeID).Contact().Create().SetSource(contact.SourceManual).SetEmail("fed@example.com").Exec(t.Context())
			})

			peak, err := s.Usage().Query().Where(entusage.Metric(string(usages.Contacts))).Only(t.Context())
			require.NoError(t, err)
			assert.Equal(t, int64(base+1), peak.Value)
		})
	}
}
