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
	"github.com/getsphericon/sphericon/internal/lifecycle"
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

func verifier(t *testing.T, env *testhelper.TestEnv, zones map[string]mockdns.Zone) *trackingdomain.Verifier {
	t.Helper()
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

	state, err := verifier(t, env, pointingAtEdge("track.pending.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
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
			state, err := verifier(t, env, zones).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
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

	state, err := verifier(t, env, nil).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStatePending, state)
}

func TestCheck_servingStaysServingWhileCnameHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(t, env, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
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

	state, err := verifier(t, env, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingClaimID)
	require.ErrorIs(t, err, trackingdomain.ErrHostTaken)
	assert.Equal(t, sendingdomain.TrackingStatePending, state)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingClaimID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStatePending, row.TrackingState)
	assert.True(t, clock.Equal(*row.TrackingCheckedAt))
	require.NotNil(t, row.TrackingReason, "the owner of the claim is told why it stays pending")
	assert.Equal(t, sendingdomain.TrackingReasonHostTaken, *row.TrackingReason)

	// The holder itself is never held back by anyone.
	held, err := env.DB.Scoped(fixtures.UmbrellaID).SendingDomain().Get(ctx, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Nil(t, held.TrackingReason)
}

// The reason goes away with the next check that does not lose the host: here the CNAME
// is gone, so the claim is plain pending again.
func TestCheck_hostTakenReasonIsClearedWhenTheCnameIsAbsent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.WayneID)

	_, err := verifier(t, env, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingClaimID)
	require.ErrorIs(t, err, trackingdomain.ErrHostTaken)

	state, err := verifier(t, env, nil).Check(ctx, s, fixtures.SendingDomainTrackingClaimID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStatePending, state)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingClaimID)
	require.NoError(t, err)
	assert.Nil(t, row.TrackingReason)
}

// An unverified claim never blocks the real owner: the pending claim sits on the host
// while Umbrella's own row is verified and keeps being re-verified.
func TestCheck_unverifiedClaimDoesNotBlockOwner(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(t, env, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, state)
}

const dnsHost = "track.dns.umbrella.test"

func TestCheck_answeringProbeMakesHostServingAndPublishesEvent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	v := probing(verifier(t, env, pointingAtEdge(dnsHost)), answering(dnsHost))
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

			v := probing(verifier(t, env, pointingAtEdge(dnsHost)), answer)
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
	state, err := verifier(t, env, pointingAtEdge("track.pending.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, state)
}

func TestCheck_servingHostThatStopsAnsweringFallsBackToDNSVerified(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	v := probing(verifier(t, env, pointingAtEdge("track.serving.umbrella.test")), probeAnswer{err: errors.New("timeout")})
	state, err := v.Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateDNSVerified, state)

	changes := env.OutboxEvents(t, events.NameTrackingDomainChanged)
	require.Len(t, changes, 1)
	assert.Equal(t, "serving", changes[0].(*events.TrackingDomainChanged).From)
	assert.Equal(t, "dns_verified", changes[0].(*events.TrackingDomainChanged).To)
}

// Two checks of one host read the same row. The one whose answer lands second was judged on
// a state that is gone: it loses the compare-and-swap, and the row keeps the state the first
// stored, which a later read agrees with.
func TestCheck_concurrentChecksEndInAStateALaterReadAgreesWith(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	read := make(chan struct{})
	release := make(chan struct{})
	slow := verifier(t, env, nil) // its DNS answer is gone: a demotion to pending
	inner := slow.Lookup
	slow.Lookup = func(ctx context.Context, host string) (string, error) {
		close(read) // the row is already read when the lookup starts
		<-release
		return inner(ctx, host)
	}

	type outcome struct {
		state sendingdomain.TrackingState
		err   error
	}
	loser := make(chan outcome)
	go func() {
		state, err := slow.Check(ctx, s, fixtures.SendingDomainTrackingDNSVerifiedID)
		loser <- outcome{state, err}
	}()
	<-read

	winner := probing(verifier(t, env, pointingAtEdge(dnsHost)), answering(dnsHost))
	state, err := winner.Check(ctx, s, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	require.Equal(t, sendingdomain.TrackingStateServing, state)

	close(release)
	got := <-loser
	require.ErrorIs(t, got.err, lifecycle.ErrInvalidTransition)

	row, err := s.SendingDomain().Get(ctx, fixtures.SendingDomainTrackingDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, row.TrackingState)
	assert.Len(t, env.OutboxEvents(t, events.NameTrackingDomainChanged), 1)
}

// The lifecycle lists six edges: each of the three states moves to both others, and the Check
// takes every one of them from what DNS and the probe answer.
func TestCheck_everyEdgeOfTheTrackingLifecycle(t *testing.T) {
	const (
		pending  = sendingdomain.TrackingStatePending
		verified = sendingdomain.TrackingStateDNSVerified
		serving  = sendingdomain.TrackingStateServing
		pendHost = "track.pending.umbrella.test"
		servHost = "track.serving.umbrella.test"
	)
	down := probeAnswer{err: errors.New("down")}
	cases := []struct {
		name     string
		id       int64
		zones    map[string]mockdns.Zone
		probe    probeAnswer
		from, to sendingdomain.TrackingState
	}{
		{"pending to dns_verified", fixtures.SendingDomainTrackingPendingID, pointingAtEdge(pendHost), down, pending, verified},
		{"pending to serving", fixtures.SendingDomainTrackingPendingID, pointingAtEdge(pendHost), answering(pendHost), pending, serving},
		{"dns_verified to pending", fixtures.SendingDomainTrackingDNSVerifiedID, nil, down, verified, pending},
		{"dns_verified to serving", fixtures.SendingDomainTrackingDNSVerifiedID, pointingAtEdge(dnsHost), answering(dnsHost), verified, serving},
		{"serving to pending", fixtures.SendingDomainTrackingServingID, nil, down, serving, pending},
		{"serving to dns_verified", fixtures.SendingDomainTrackingServingID, pointingAtEdge(servHost), down, serving, verified},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := context.Background()
			s := env.DB.Scoped(fixtures.UmbrellaID)

			state, err := probing(verifier(t, env, c.zones), c.probe).Check(ctx, s, c.id)
			require.NoError(t, err)
			assert.Equal(t, c.to, state)

			row, err := s.SendingDomain().Get(ctx, c.id)
			require.NoError(t, err)
			assert.Equal(t, c.to, row.TrackingState)
			assert.True(t, clock.Equal(*row.TrackingStateChangedAt))

			changes := env.OutboxEvents(t, events.NameTrackingDomainChanged)
			require.Len(t, changes, 1)
			assert.Equal(t, string(c.from), changes[0].(*events.TrackingDomainChanged).From)
			assert.Equal(t, string(c.to), changes[0].(*events.TrackingDomainChanged).To)
		})
	}
}
