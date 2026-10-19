package trackingdomain_test

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
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
)

const edge = "edge.example.com"

var clock = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

const secret = "probe-secret"

// probeAnswer is a Doer that answers every probe with one status and body.
type probeAnswer struct {
	status int
	body   string
	err    error
}

func (p probeAnswer) Do(req *http.Request) (*http.Response, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &http.Response{StatusCode: p.status, Body: io.NopCloser(strings.NewReader(p.body)), Request: req}, nil
}

// answering is the probe a healthy edge gives: 200 and the value for the host asked.
func answering(host string) probeAnswer {
	return probeAnswer{status: http.StatusOK, body: trackingdomain.ProbeValue(secret, host)}
}

func verifier(env *testhelper.TestEnv, t *testing.T, zones map[string]mockdns.Zone) *trackingdomain.Verifier {
	return &trackingdomain.Verifier{
		Lookup: dnstest.Resolver(t, zones).LookupCNAME,
		Target: edge,
		Now:    func() time.Time { return clock },
		Secret: secret,
		Events: env.Bus,
	}
}

func probing(v *trackingdomain.Verifier, d trackingdomain.Doer) *trackingdomain.Verifier {
	v.HTTP = d
	return v
}

func pointingAtEdge(host string) map[string]mockdns.Zone {
	return map[string]mockdns.Zone{host + ".": {CNAME: edge + "."}}
}

func TestCheck_cnameAtEdgeVerifiesDNS(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(env, t, pointingAtEdge("track.pending.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, state)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingPendingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)
	assert.True(t, clock.Equal(*row.TrackingCheckedAt))
	assert.True(t, clock.Equal(*row.TrackingStateChangedAt))
}

func TestCheck_missingOrWrongCnameStaysPending(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	for name, zones := range map[string]map[string]mockdns.Zone{
		"missing": nil,
		"wrong":   {"track.pending.umbrella.test.": {CNAME: "elsewhere.example.net."}},
	} {
		t.Run(name, func(t *testing.T) {
			state, err := verifier(env, t, zones).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
			require.NoError(t, err)
			assert.Equal(t, sendingdomain.TrackingStatePending, state)

			row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingPendingID)
			require.NoError(t, err)
			assert.Equal(t, sendingdomain.TrackingStatePending, row.TrackingState)
			assert.True(t, clock.Equal(*row.TrackingCheckedAt))
			assert.Nil(t, row.TrackingStateChangedAt)
		})
	}
}

func TestCheck_lostCnameFallsBackToPending(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(env, t, nil).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStatePending, state)
}

func TestCheck_servingStaysServingWhileCnameHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(env, t, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, state)
}

func TestCheck_resolverErrorChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	v := &trackingdomain.Verifier{
		Lookup: func(context.Context, string) (string, error) { return "", errors.New("servfail") },
		Target: edge,
		Now:    func() time.Time { return clock },
		Events: env.Bus,
	}
	_, err := v.Check(ctx, s, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.Error(t, err)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, row.TrackingState)
	assert.False(t, clock.Equal(*row.TrackingCheckedAt))
}

// Umbrella holds serving.umbrella.test; Wayne's claim on the same name cannot verify
// while Umbrella is verified, and stays pending.
func TestCheck_secondWorkspaceCannotVerifyHeldHost(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.WayneID)

	state, err := verifier(env, t, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingClaimID)
	require.ErrorIs(t, err, trackingdomain.ErrHostTaken)
	assert.Equal(t, sendingdomain.TrackingStatePending, state)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingClaimID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStatePending, row.TrackingState)
	assert.True(t, clock.Equal(*row.TrackingCheckedAt))
}

// An unverified claim never blocks the real owner: the pending claim sits on the host
// while Umbrella's own row is verified and keeps being re-verified.
func TestCheck_unverifiedClaimDoesNotBlockOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(env, t, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, state)
}

const dnsHost = "track.dns.umbrella.test"

func TestCheck_answeringProbeMakesHostServingAndPublishesEvent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	v := probing(verifier(env, t, pointingAtEdge(dnsHost)), answering(dnsHost))
	state, err := v.Check(ctx, s, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, state)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, row.TrackingState)
	assert.True(t, clock.Equal(*row.TrackingStateChangedAt))

	changes := env.OutboxEvents(t, events.NameTrackingDomainChanged)
	require.Len(t, changes, 1)
	change := changes[0].(*events.TrackingDomainChanged)
	assert.Equal(t, dnsHost, change.Host)
	assert.Equal(t, "dns_verified", change.From)
	assert.Equal(t, "serving", change.To)
	assert.EqualValues(t, fixtures.UmbrellaID, change.WorkspaceID)
}

func TestCheck_noSuccessfulProbeNeverServes(t *testing.T) {
	cases := map[string]probeAnswer{
		"unreachable":  {err: errors.New("tls handshake failure")},
		"not ok":       {status: http.StatusNotFound, body: "nope"},
		"foreign body": {status: http.StatusOK, body: "parked"},
		"other host":   {status: http.StatusOK, body: trackingdomain.ProbeValue(secret, "track.other.test")},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			s := env.DB.Scoped(fixtures.UmbrellaID)

			v := probing(verifier(env, t, pointingAtEdge(dnsHost)), answer)
			state, err := v.Check(ctx, s, fixtures.SendingDomainTrackingDNSVerifiedID)
			require.NoError(t, err)
			assert.Equal(t, sendingdomain.TrackingStateDNSVerified, state)
			assert.Empty(t, env.OutboxEvents(t, events.NameTrackingDomainChanged))
		})
	}
}

func TestCheck_pendingHostNeverSkipsToServingWithoutProbe(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	// No HTTP client: DNS alone gets a host to dns_verified, never further.
	state, err := verifier(env, t, pointingAtEdge("track.pending.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, state)
}

func TestCheck_servingHostThatStopsAnsweringFallsBackToDNSVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	v := probing(verifier(env, t, pointingAtEdge("track.serving.umbrella.test")), probeAnswer{err: errors.New("timeout")})
	state, err := v.Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, state)

	changes := env.OutboxEvents(t, events.NameTrackingDomainChanged)
	require.Len(t, changes, 1)
	assert.Equal(t, "serving", changes[0].(*events.TrackingDomainChanged).From)
	assert.Equal(t, "dns_verified", changes[0].(*events.TrackingDomainChanged).To)
}
