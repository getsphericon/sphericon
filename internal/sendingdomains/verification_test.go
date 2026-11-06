package sendingdomains_test

import (
	"context"
	"net"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/domainverification"
)

// verification is the domain verification use case over the test's database: the dev resolver's
// answers are read the way a check reads them.
func verification(env *testhelper.TestEnv, lookup domainname.TXTLookup) *domainverification.UseCase {
	return domainverification.New(sendingdomains.New(env.Bus, nil), lookup, memberships.OwnerNotifier(env.DB, env.SystemMail))
}

func TestDue_neverCheckedFirstAndArchivedSkipped(t *testing.T) {
	env := testhelper.Setup(t)

	// Fixtures: the verified domain has last_checked_at set; the unverified one is never
	// checked (NULL). NULLS FIRST must surface it ahead.
	ids, err := sendingdomains.Due(context.Background(), env.DB, 100)
	require.NoError(t, err)
	require.Contains(t, ids, int64(fixtures.SendingDomainUnverifiedID))
	require.Contains(t, ids, int64(fixtures.SendingDomainVerifiedID))
	assert.Less(t, slices.Index(ids, int64(fixtures.SendingDomainUnverifiedID)), slices.Index(ids, int64(fixtures.SendingDomainVerifiedID)),
		"a never-checked domain is re-checked before an already-checked one")
	assert.NotContains(t, ids, int64(fixtures.SendingDomainArchivedID))
}

func TestDevTXTLookup_trustsSeededDomains(t *testing.T) {
	env := testhelper.Setup(t)

	ok, err := verification(env, sendingdomains.DevTXTLookup(env.DB)).VerifyDKIM(context.Background(), env.DB.Scoped(fixtures.AcmeID), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	assert.True(t, ok, "dev lookup makes a seeded domain verify")
}

// The same lookup answers a Join domain's proof record from the stored token, so a domain an
// Owner adds verifies in dev and e2e without DNS; a domain nobody claims is NXDOMAIN.
func TestDevTXTLookup_answersAJoinDomainsRecordFromItsToken(t *testing.T) {
	env := testhelper.Setup(t)
	pending, err := env.DB.JoinDomain.Get(t.Context(), fixtures.JoinDomainPendingID)
	require.NoError(t, err)

	records, err := sendingdomains.DevTXTLookup(env.DB)(t.Context(), "_sphericon-join."+pending.Domain+".")

	require.NoError(t, err)
	assert.Equal(t, []string{pending.Token}, records)
}

func TestDevTXTLookup_aJoinDomainNobodyClaimsIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := sendingdomains.DevTXTLookup(env.DB)(t.Context(), "_sphericon-join.unclaimed.test.")

	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.True(t, dnsErr.IsNotFound)
}

// The end-to-end resolver answers a Sending domain's DMARC record with the suggested one, so a
// domain a scenario adds is bulk-ready too.
func TestDevTXTLookup_answersASendingDomainsDMARCRecord(t *testing.T) {
	env := testhelper.Setup(t)

	err := verification(env, sendingdomains.DevTXTLookup(env.DB)).Recheck(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(t.Context(), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.NotNil(t, row.DmarcFoundAt)
}
