package jobs_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
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

func TestVerifyTrackingDomainByID_servingHostThatStopsAnsweringNotifiesOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zones := map[string]mockdns.Zone{"track.serving.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	down := probeFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })

	err := jobs.VerifyTrackingDomainByID(ctx, env.DB, trackingVerifier(t, env, zones, down), env.SystemMail, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.UmbrellaOwnerRitaEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "track.serving.umbrella.test")
}

func TestVerifyTrackingDomainByID_promotionNotifiesNobody(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zones := map[string]mockdns.Zone{"track.dns.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	up := probeFunc(func(r *http.Request) (*http.Response, error) {
		body := trackingdomain.ProbeValue("job-secret", r.URL.Hostname())
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	err := jobs.VerifyTrackingDomainByID(ctx, env.DB, trackingVerifier(t, env, zones, up), env.SystemMail, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, row.TrackingState)
	assert.Empty(t, env.SystemMail.Messages())
}

func TestVerifySendingDomainWorker_dkimFailureStillRunsTheTrackingCheck(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	zones := map[string]mockdns.Zone{"track.serving.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	down := probeFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	servfail := func(context.Context, string) ([]string, error) { return nil, errors.New("servfail") }

	w := jobs.NewVerifySendingDomainWorkerWithTracking(env.DB, servfail, env.SystemMail, trackingVerifier(t, env, zones, down))
	err := w.Work(ctx, job(jobs.VerifySendingDomainArgs{SendingDomainID: fixtures.SendingDomainTrackingServingID}))
	require.Error(t, err, "the DKIM lookup error is still reported")

	row, err := env.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)
}

func TestRecheckTrackingDomainsWorker_demotesAServingHostThatStopsAnswering(t *testing.T) {
	e := newRiverEnv(t)
	ctx := e.workCtx()

	require.NoError(t, jobs.NewRecheckTrackingDomainsWorker(e.DB).Work(ctx, job(jobs.RecheckTrackingDomainsArgs{})))
	kinds := e.queued(t)
	require.NotEmpty(t, kinds)
	for _, k := range kinds {
		assert.Equal(t, "tracking_domain_verify", k)
	}

	zones := map[string]mockdns.Zone{"track.serving.umbrella.test.": {CNAME: testhelper.TrackingCNAMETarget + "."}}
	down := probeFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") })
	w := jobs.NewVerifyTrackingDomainWorker(e.DB, e.SystemMail, trackingVerifier(t, e.TestEnv, zones, down))
	require.NoError(t, w.Work(ctx, job(jobs.VerifyTrackingDomainArgs{SendingDomainID: fixtures.SendingDomainTrackingServingID})))

	row, err := e.DB.SendingDomain.Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)
}
