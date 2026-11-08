package sendingdomains_test

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/dkimstate"
	"github.com/getsphericon/sphericon/internal/kernel/domainname"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// domains is the module over the test's database, its checks resolving through lookup.
func domains(env *testhelper.TestEnv, lookup domainname.TXTLookup) *sendingdomains.Module {
	return sendingdomains.New(env.Bus, nil, lookup, time.Now)
}

// acme is the scope of the Workspace the checked fixture domains belong to.
func acme(env *testhelper.TestEnv) *ent.Scoped { return env.DB.Scoped(fixtures.AcmeID) }

// dkimLookup reports whether name is a DKIM record's, the lookup a check makes before its DMARC
// ones.
func dkimLookup(name string) bool { return strings.Contains(name, "._domainkey.") }

func TestDue_neverCheckedFirstAndArchivedSkipped(t *testing.T) {
	env := testhelper.Setup(t)

	// Fixtures: the verified domain has last_checked_at set; the unverified one is never
	// checked (NULL). NULLS FIRST must surface it ahead.
	due, err := sendingdomains.Due(context.Background(), env.DB, 100)
	require.NoError(t, err)
	ids := lo.Map(due, func(d *ent.SendingDomain, _ int) int64 { return d.ID })
	require.Contains(t, ids, int64(fixtures.SendingDomainUnverifiedID))
	require.Contains(t, ids, int64(fixtures.SendingDomainVerifiedID))
	assert.Less(t, slices.Index(ids, int64(fixtures.SendingDomainUnverifiedID)), slices.Index(ids, int64(fixtures.SendingDomainVerifiedID)),
		"a never-checked domain is re-checked before an already-checked one")
	assert.NotContains(t, ids, int64(fixtures.SendingDomainArchivedID))
}

func TestDevTXTLookup_trustsSeededDomains(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := domains(env, sendingdomains.DevTXTLookup(env.DB)).Verify(context.Background(), acme(env), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	assert.True(t, dkimstate.Is(row), "dev lookup makes a seeded domain verify")
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

	err := domains(env, sendingdomains.DevTXTLookup(env.DB)).Recheck(t.Context(), acme(env), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(t.Context(), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.NotNil(t, row.DmarcFoundAt)
}

func TestVerify_becomesVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	require.False(t, dkimstate.Is(dom))

	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.news.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT
	row, err := domains(env, lookup).Verify(ctx, acme(env), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(row))

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(reloaded))
	require.NotNil(t, reloaded.VerifiedAt)
	assert.Nil(t, reloaded.VerificationLostAt)
	require.NotNil(t, reloaded.LastCheckedAt)
	assert.Empty(t, env.SystemMail.Messages(), "becoming verified notifies nobody")
}

func TestVerify_flipsToUnverifiedWhenRecordGoneAndEmailsOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT

	row, err := domains(env, gone).Verify(ctx, acme(env), fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(row))

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(reloaded), "a verified domain loses verification when its DKIM DNS is gone")
	require.NotNil(t, reloaded.VerificationLostAt, "the loss is visible in the data")
	require.NotNil(t, reloaded.VerifiedAt, "when it was verified is kept")
	lostAt := *reloaded.VerificationLostAt

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Equal(t, "Action required: sending domain mail.acme.com is no longer verified", msgs[0].Subject)
	assert.Contains(t, msgs[0].Text, "sphericon._domainkey.mail.acme.com")
	assert.Contains(t, msgs[0].Text, "is now blocked")

	// The flip self-clears: the next check finds it already unverified and tells nobody again.
	err = domains(env, gone).Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.Len(t, env.SystemMail.Messages(), 1)
	again, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.True(t, lostAt.Equal(*again.VerificationLostAt), "a repeated check keeps the first loss moment")
}

func TestVerify_verifiedAgainClearsTheLoss(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	err = domains(env, dnstest.Resolver(t, nil).LookupTXT).Recheck(ctx, acme(env), dom.ID)
	require.NoError(t, err)

	back := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT
	verified, err := domains(env, back).Verify(ctx, acme(env), dom.ID)

	require.NoError(t, err)
	assert.True(t, dkimstate.Is(verified))
	row, err := env.DB.SendingDomain.Get(ctx, dom.ID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(row))
	assert.Nil(t, row.VerificationLostAt)
	assert.True(t, row.VerifiedAt.After(*dom.VerifiedAt), "verified_at moves to the new verification")
}

func TestVerify_aFailingNoticeNeverFailsTheCheck(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.SystemMail.SetErr(errors.New("smtp down"))

	checked, err := domains(env, dnstest.Resolver(t, nil).LookupTXT).Verify(ctx, acme(env), fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	assert.False(t, dkimstate.Is(checked))
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(row))
	assert.NotNil(t, row.VerificationLostAt)
}

func TestRecheck_withoutASystemSenderStillFlips(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutSystemSender())
	ctx := context.Background()
	m := sendingdomains.New(env.Bus, nil, dnstest.Resolver(t, nil).LookupTXT, time.Now)

	err := m.Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(row))
	assert.NotNil(t, row.VerificationLostAt)
}

func TestRecheck_unknownDomainIsAnError(t *testing.T) {
	env := testhelper.Setup(t)

	err := domains(env, dnstest.Resolver(t, nil).LookupTXT).Recheck(context.Background(), acme(env), 424242)

	require.ErrorIs(t, err, sendingdomains.ErrNotFound)
}

func TestRecheck_resolverErrorDoesNotChangeState(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {Err: errors.New("servfail")}}).LookupTXT

	err := domains(env, lookup).Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)
	require.Error(t, err)

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(reloaded), "a transient resolver failure must not flip verified")
}

// A check judged on a row another check already changed is stale: the lookup runs between the
// check's read and its write, so a second check that flips the domain meanwhile is a concurrent
// one. The late check neither rewrites the row nor tells the Owners a second time.
func TestRecheck_staleCheckDoesNotReportASecondFlip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT
	concurrent := func(ctx context.Context, name string) ([]string, error) {
		if dkimLookup(name) {
			err := domains(env, gone).Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)
			require.NoError(t, err)
		}
		return gone(ctx, name)
	}

	err := domains(env, concurrent).Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	assert.Len(t, env.SystemMail.Messages(), 1, "the owner is told once, by the check that won")
}

// A check looks the DMARC record up next to DKIM and keeps when it found one: publishing the
// record makes the domain bulk-ready, withdrawing it clears that, and a record at the
// organizational domain counts as one at the domain.
func TestRecheck_aPublishedDMARCRecordMakesTheDomainBulkReady(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	zone := dnstest.NewZone(t)
	m := domains(env, zone.LookupTXT)
	// check runs a check of news.acme.com and reads when its DMARC record was found.
	check := func() *time.Time {
		t.Helper()
		err := m.Recheck(ctx, acme(env), fixtures.SendingDomainUnverifiedID)
		require.NoError(t, err)
		row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
		require.NoError(t, err)
		return row.DmarcFoundAt
	}

	none := check()
	assert.Nil(t, none, "news.acme.com publishes no DMARC record")
	looked, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.NotNil(t, looked.DmarcCheckedAt, "a lookup that found nothing is still a lookup")

	zone.PublishTXT(t, "_dmarc.news.acme.com.", "v=DMARC1; p=none;")
	first := check()
	require.NotNil(t, first)

	again := check()
	require.NotNil(t, again)
	assert.Equal(t, *first, *again, "a repeated check keeps the moment it was first found")

	zone.WithdrawTXT("_dmarc.news.acme.com.")
	withdrawn := check()
	assert.Nil(t, withdrawn, "a check that finds no record clears it")

	zone.PublishTXT(t, "_dmarc.acme.com.", "v=DMARC1; p=quarantine;")
	organizational := check()
	assert.NotNil(t, organizational, "the organizational domain's record covers news.acme.com")
}

// A failed DMARC lookup changes nothing: neither what the domain's DMARC was known to be, nor its
// DKIM result, nor the check, which still succeeds.
func TestRecheck_aFailedDMARCLookupChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	require.NotNil(t, before.DmarcFoundAt)
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{
		"sphericon._domainkey.mail.acme.com.": {TXT: []string{before.DkimPublicKey}},
		"_dmarc.mail.acme.com.":               {Err: errors.New("servfail")},
	}).LookupTXT

	err = domains(env, lookup).Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	after, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	require.NotNil(t, after.DmarcFoundAt)
	assert.Equal(t, *before.DmarcFoundAt, *after.DmarcFoundAt)
	assert.Equal(t, before.DmarcCheckedAt, after.DmarcCheckedAt, "a failed lookup is not a lookup")
	verified := dkimstate.Is(after)
	assert.True(t, verified)
	require.NotNil(t, after.LastCheckedAt)
	assert.NotEqual(t, *before.LastCheckedAt, *after.LastCheckedAt, "the DKIM check was stored")
}

// A domain whose DMARC lookups have only failed was never looked up: its DKIM check is stored, but
// nothing says its DMARC record is missing.
func TestRecheck_aFirstDMARCLookupThatFailsLeavesTheDomainUnjudged(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{
		"_dmarc.news.acme.com.": {Err: errors.New("servfail")},
	}).LookupTXT

	err := domains(env, lookup).Recheck(ctx, acme(env), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.NotNil(t, row.LastCheckedAt, "the DKIM check was stored")
	assert.Nil(t, row.DmarcCheckedAt)
	assert.Nil(t, row.DmarcFoundAt)
}

// A check queued before its domain was deleted checks again when it runs (ADR 0099): a deleted
// domain is not kept working, so the check stores nothing and tells no Owner it lost verification.
func TestRecheck_aDeletedDomainChangesNothingAndNotifiesNoOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainArchivedID)
	require.NoError(t, err)
	require.True(t, dkimstate.Is(before), "the deleted fixture domain was verified")

	err = domains(env, dnstest.Resolver(t, nil).LookupTXT).Recheck(ctx, acme(env), fixtures.SendingDomainArchivedID)

	require.NoError(t, err)
	after, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainArchivedID)
	require.NoError(t, err)
	assert.Equal(t, before.LastCheckedAt, after.LastCheckedAt)
	assert.Equal(t, before.VerificationLostAt, after.VerificationLostAt)
	assert.Equal(t, before.DmarcCheckedAt, after.DmarcCheckedAt)
	assert.Empty(t, env.SystemMail.Messages())
}

// A domain deleted while its DNS is being looked up is not judged by the check either.
func TestRecheck_aDomainDeletedDuringTheCheckNotifiesNoOne(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	gone := dnstest.Resolver(t, nil).LookupTXT
	deleting := func(ctx context.Context, name string) ([]string, error) {
		if dkimLookup(name) {
			env.DB.SendingDomain.UpdateOneID(fixtures.SendingDomainVerifiedID).SetDiscardedAt(time.Now()).ExecX(ctx)
		}
		return gone(ctx, name)
	}

	err := domains(env, deleting).Recheck(ctx, acme(env), fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.Nil(t, row.VerificationLostAt)
	assert.Empty(t, env.SystemMail.Messages())
}
