package domainverify_test

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func module(env *testhelper.TestEnv, lookup sending.TXTLookup) *domainverify.Module {
	return domainverify.New(env.DB, lookup, accounts.OwnerNotifier(env.DB, env.SystemMail))
}

func TestVerifyDKIM_becomesVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	require.False(t, dkimstate.Is(dom))

	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.news.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT
	ok, err := module(env, lookup).VerifyDKIM(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, ok)

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(reloaded))
	require.NotNil(t, reloaded.VerifiedAt)
	assert.Nil(t, reloaded.VerificationLostAt)
	require.NotNil(t, reloaded.LastCheckedAt)
	assert.Empty(t, env.SystemMail.Messages(), "becoming verified notifies nobody")
}

func TestVerifyDKIM_flipsToUnverifiedWhenRecordGoneAndEmailsOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT

	ok, err := module(env, gone).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, ok)

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
	_, err = module(env, gone).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.Len(t, env.SystemMail.Messages(), 1)
	again, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.True(t, lostAt.Equal(*again.VerificationLostAt), "a repeated check keeps the first loss moment")
}

func TestVerifyDKIM_verifiedAgainClearsTheLoss(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	_, err = module(env, dnstest.Resolver(t, nil).LookupTXT).VerifyDKIM(ctx, dom.ID)
	require.NoError(t, err)

	back := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT
	ok, err := module(env, back).VerifyDKIM(ctx, dom.ID)

	require.NoError(t, err)
	assert.True(t, ok)
	row, err := env.DB.SendingDomain.Get(ctx, dom.ID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(row))
	assert.Nil(t, row.VerificationLostAt)
	assert.True(t, row.VerifiedAt.After(*dom.VerifiedAt), "verified_at moves to the new verification")
}

func TestVerifyDKIM_aFailingNoticeNeverFailsTheCheck(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.SystemMail.SetErr(errors.New("smtp down"))

	ok, err := module(env, dnstest.Resolver(t, nil).LookupTXT).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	assert.False(t, ok)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(row))
	assert.NotNil(t, row.VerificationLostAt)
}

func TestVerifyDKIM_withoutANotifierStillFlips(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	m := domainverify.New(env.DB, dnstest.Resolver(t, nil).LookupTXT, nil)

	_, err := m.VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(row))
	assert.NotNil(t, row.VerificationLostAt)
}

func TestVerifyDKIM_unknownDomainIsAnError(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := module(env, dnstest.Resolver(t, nil).LookupTXT).VerifyDKIM(context.Background(), 424242)

	require.Error(t, err)
}

func TestVerifyDKIM_resolverErrorDoesNotChangeState(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {Err: errors.New("servfail")}}).LookupTXT

	_, err := module(env, lookup).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
	require.Error(t, err)

	reloaded, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.True(t, dkimstate.Is(reloaded), "a transient resolver failure must not flip verified")
}

// A check judged on a row another check already changed is stale: the lookup runs between the
// check's read and its write, so a second check that flips the domain meanwhile is a concurrent
// one. The late check neither rewrites the row nor tells the Owners a second time.
func TestVerifyDKIM_staleCheckDoesNotReportASecondFlip(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	gone := dnstest.Resolver(t, nil).LookupTXT
	concurrent := func(ctx context.Context, name string) ([]string, error) {
		_, err := module(env, gone).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
		require.NoError(t, err)
		return gone(ctx, name)
	}

	_, err := module(env, concurrent).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	assert.Len(t, env.SystemMail.Messages(), 1, "the owner is told once, by the check that won")
}

func TestDue_neverCheckedFirstAndArchivedSkipped(t *testing.T) {
	env := testhelper.Setup(t)

	// Fixtures: the verified domain has last_checked_at set; the unverified one is never
	// checked (NULL). NULLS FIRST must surface it ahead.
	ids, err := module(env, nil).Due(context.Background(), 100)
	require.NoError(t, err)
	require.Contains(t, ids, int64(fixtures.SendingDomainUnverifiedID))
	require.Contains(t, ids, int64(fixtures.SendingDomainVerifiedID))
	assert.Less(t, slices.Index(ids, int64(fixtures.SendingDomainUnverifiedID)), slices.Index(ids, int64(fixtures.SendingDomainVerifiedID)),
		"a never-checked domain is re-checked before an already-checked one")
	assert.NotContains(t, ids, int64(fixtures.SendingDomainArchivedID))
}

func TestDevTXTLookup_trustsSeededDomains(t *testing.T) {
	env := testhelper.Setup(t)

	ok, err := module(env, domainverify.DevTXTLookup(env.DB)).VerifyDKIM(context.Background(), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	assert.True(t, ok, "dev lookup makes a seeded domain verify")
}

// The same lookup answers a Join domain's proof record from the stored token, so a domain an
// Owner adds verifies in dev and e2e without DNS; a domain nobody claims is NXDOMAIN.
func TestDevTXTLookup_answersAJoinDomainsRecordFromItsToken(t *testing.T) {
	env := testhelper.Setup(t)
	pending, err := env.DB.JoinDomain.Get(t.Context(), fixtures.JoinDomainPendingID)
	require.NoError(t, err)

	records, err := domainverify.DevTXTLookup(env.DB)(t.Context(), "_sphericon-join."+pending.Domain+".")

	require.NoError(t, err)
	assert.Equal(t, []string{pending.Token}, records)
}

func TestDevTXTLookup_aJoinDomainNobodyClaimsIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := domainverify.DevTXTLookup(env.DB)(t.Context(), "_sphericon-join.unclaimed.test.")

	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.True(t, dnsErr.IsNotFound)
}

// A check looks the DMARC record up next to DKIM and keeps when it found one: publishing the
// record makes the domain bulk-ready, withdrawing it clears that, and a record at the
// organizational domain counts as one at the domain.
func TestVerify_aPublishedDMARCRecordMakesTheDomainBulkReady(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	zone := dnstest.NewZone(t)
	m := module(env, zone.LookupTXT)
	// check runs a check of news.acme.com and reads when its DMARC record was found.
	check := func() *time.Time {
		t.Helper()
		err := m.Verify(ctx, fixtures.SendingDomainUnverifiedID)
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
func TestVerify_aFailedDMARCLookupChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	before, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	require.NotNil(t, before.DmarcFoundAt)
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{
		"sphericon._domainkey.mail.acme.com.": {TXT: []string{before.DkimPublicKey}},
		"_dmarc.mail.acme.com.":               {Err: errors.New("servfail")},
	}).LookupTXT

	err = module(env, lookup).Verify(ctx, fixtures.SendingDomainVerifiedID)

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
func TestVerify_aFirstDMARCLookupThatFailsLeavesTheDomainUnjudged(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{
		"_dmarc.news.acme.com.": {Err: errors.New("servfail")},
	}).LookupTXT

	err := module(env, lookup).Verify(ctx, fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.NotNil(t, row.LastCheckedAt, "the DKIM check was stored")
	assert.Nil(t, row.DmarcCheckedAt)
	assert.Nil(t, row.DmarcFoundAt)
}

// The end-to-end resolver answers a Sending domain's DMARC record with the suggested one, so a
// domain a scenario adds is bulk-ready too.
func TestDevTXTLookup_answersASendingDomainsDMARCRecord(t *testing.T) {
	env := testhelper.Setup(t)

	err := module(env, domainverify.DevTXTLookup(env.DB)).Verify(t.Context(), fixtures.SendingDomainUnverifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(t.Context(), fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	assert.NotNil(t, row.DmarcFoundAt)
}
