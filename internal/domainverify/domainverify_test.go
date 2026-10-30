package domainverify_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/dkimstate"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/sending"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
)

type probeFunc func(*http.Request) (*http.Response, error)

func (f probeFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func trackingVerifier(t *testing.T, env *testhelper.TestEnv, zones map[string]mockdns.Zone, doer trackingdomain.Doer) *trackingdomain.Verifier {
	t.Helper()
	return &trackingdomain.Verifier{
		Lookup: dnstest.Resolver(t, zones).LookupCNAME,
		Target: testhelper.TrackingCNAMETarget,
		Now:    func() time.Time { return time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC) },
		HTTP:   doer,
		Secret: "job-secret",
		Events: env.Bus,
	}
}

// module builds the module over the system mailbox, the way the composition roots do.
func module(env *testhelper.TestEnv, lookup sending.TXTLookup, tracking *trackingdomain.Verifier) *domainverify.Module {
	return domainverify.New(env.DB, lookup, tracking, accounts.OwnerNotifier(env.DB, env.SystemMail))
}

func TestVerifyDKIM_becomesVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	dom, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainUnverifiedID)
	require.NoError(t, err)
	require.False(t, dkimstate.Is(dom))

	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.news.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT
	ok, err := module(env, lookup, nil).VerifyDKIM(ctx, fixtures.SendingDomainUnverifiedID)
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

	ok, err := module(env, gone, nil).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
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
	_, err = module(env, gone, nil).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
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
	_, err = module(env, dnstest.Resolver(t, nil).LookupTXT, nil).VerifyDKIM(ctx, dom.ID)
	require.NoError(t, err)

	back := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {TXT: []string{dom.DkimPublicKey}}}).LookupTXT
	ok, err := module(env, back, nil).VerifyDKIM(ctx, dom.ID)

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

	ok, err := module(env, dnstest.Resolver(t, nil).LookupTXT, nil).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)

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
	m := domainverify.New(env.DB, dnstest.Resolver(t, nil).LookupTXT, nil, nil)

	_, err := m.VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainVerifiedID)
	require.NoError(t, err)
	assert.False(t, dkimstate.Is(row))
	assert.NotNil(t, row.VerificationLostAt)
}

func TestVerifyDKIM_unknownDomainIsAnError(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := module(env, dnstest.Resolver(t, nil).LookupTXT, nil).VerifyDKIM(context.Background(), 424242)

	require.Error(t, err)
}

func TestVerifyDKIM_resolverErrorDoesNotChangeState(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	lookup := dnstest.Resolver(t, map[string]mockdns.Zone{"sphericon._domainkey.mail.acme.com.": {Err: errors.New("servfail")}}).LookupTXT

	_, err := module(env, lookup, nil).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
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
		_, err := module(env, gone, nil).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)
		require.NoError(t, err)
		return gone(ctx, name)
	}

	_, err := module(env, concurrent, nil).VerifyDKIM(ctx, fixtures.SendingDomainVerifiedID)

	require.NoError(t, err)
	assert.Len(t, env.SystemMail.Messages(), 1, "the owner is told once, by the check that won")
}

func TestDue_neverCheckedFirstAndArchivedSkipped(t *testing.T) {
	env := testhelper.Setup(t)

	// Fixtures: the verified domain has last_checked_at set; the unverified one is never
	// checked (NULL). NULLS FIRST must surface it ahead.
	ids, err := module(env, nil, nil).Due(context.Background(), 100)
	require.NoError(t, err)
	require.Contains(t, ids, int64(fixtures.SendingDomainUnverifiedID))
	require.Contains(t, ids, int64(fixtures.SendingDomainVerifiedID))
	assert.Less(t, slices.Index(ids, int64(fixtures.SendingDomainUnverifiedID)), slices.Index(ids, int64(fixtures.SendingDomainVerifiedID)),
		"a never-checked domain is re-checked before an already-checked one")
	assert.NotContains(t, ids, int64(fixtures.SendingDomainArchivedID))
}

func TestTrackingDue_servingAndDNSVerifiedHostsOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	ids, err := module(env, nil, nil).TrackingDue(ctx)
	require.NoError(t, err)

	assert.Contains(t, ids, int64(fixtures.SendingDomainTrackingServingID))
	assert.Contains(t, ids, int64(fixtures.SendingDomainTrackingDNSVerifiedID))
	assert.NotContains(t, ids, int64(fixtures.SendingDomainArchivedID), "an archived domain serves nothing to demote")
	due, err := env.DB.SendingDomain.Query().
		Where(sendingdomain.TrackingStateIn(sendingdomain.TrackingStateDNSVerified, sendingdomain.TrackingStateServing), sendingdomain.IDNEQ(fixtures.SendingDomainArchivedID)).
		Count(ctx)
	require.NoError(t, err)
	assert.Len(t, ids, due)
}

func TestVerifyTracking_servingHostThatStopsAnsweringNotifiesOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zones := map[string]mockdns.Zone{"track.serving.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	down := probeFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })

	err := module(env, nil, trackingVerifier(t, env, zones, down)).VerifyTracking(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.UmbrellaOwnerRitaEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "track.serving.umbrella.test")
}

func TestVerifyTracking_promotionNotifiesNobody(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zones := map[string]mockdns.Zone{"track.dns.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	up := probeFunc(func(r *http.Request) (*http.Response, error) {
		body := trackingdomain.ProbeValue("job-secret", r.URL.Hostname())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	err := module(env, nil, trackingVerifier(t, env, zones, up)).VerifyTracking(ctx, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, row.TrackingState)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestVerifyTracking_featureOffDoesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, module(env, nil, nil).VerifyTracking(ctx, fixtures.SendingDomainTrackingServingID))

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, row.TrackingState)
}

func TestVerifyTracking_staleCheckLosesWithoutErrorOrNotice(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.FeedDB.Scoped(fixtures.UmbrellaID)

	v := trackingVerifier(t, env, nil, nil)
	lookup := v.Lookup
	v.Lookup = func(ctx context.Context, host string) (string, error) {
		// A concurrent check moves the row after this one read it.
		moved := s.SendingDomain().UpdateOneID(fixtures.SendingDomainTrackingServingID).
			SetTrackingState(sendingdomain.TrackingStateDNSVerified).Exec(ctx)
		require.NoError(t, moved)
		return lookup(ctx, host)
	}
	require.NoError(t, module(env, nil, v).VerifyTracking(ctx, fixtures.SendingDomainTrackingServingID))

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestVerify_dkimFailureStillRunsTheTrackingCheck(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zones := map[string]mockdns.Zone{"track.serving.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	down := probeFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	servfail := func(context.Context, string) ([]string, error) { return nil, errors.New("servfail") }

	err := module(env, servfail, trackingVerifier(t, env, zones, down)).Verify(ctx, fixtures.SendingDomainTrackingServingID)
	require.Error(t, err, "the DKIM lookup error is still reported")

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)
}

// DevTXTLookup echoes each domain's own stored key, so a seeded/added domain
// self-verifies in dev without real DNS (the local send-gate escape, ADR 0010).
func TestDevTXTLookup_trustsSeededDomains(t *testing.T) {
	env := testhelper.Setup(t)

	ok, err := module(env, domainverify.DevTXTLookup(env.DB), nil).VerifyDKIM(context.Background(), fixtures.SendingDomainUnverifiedID)

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
