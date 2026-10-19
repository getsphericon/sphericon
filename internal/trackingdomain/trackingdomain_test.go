package trackingdomain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/foxcpp/go-mockdns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/sendingdomain"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/trackingdomain"
)

const edge = "edge.example.com"

var clock = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func verifier(t *testing.T, zones map[string]mockdns.Zone) *trackingdomain.Verifier {
	return &trackingdomain.Verifier{
		Lookup: dnstest.Resolver(t, zones).LookupCNAME,
		Target: edge,
		Now:    func() time.Time { return clock },
	}
}

func pointingAtEdge(host string) map[string]mockdns.Zone {
	return map[string]mockdns.Zone{host + ".": {CNAME: edge + "."}}
}

func TestCheck_cnameAtEdgeVerifiesDNS(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(t, pointingAtEdge("track.pending.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
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
			state, err := verifier(t, zones).Check(ctx, s, fixtures.SendingDomainTrackingPendingID)
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

	state, err := verifier(t, nil).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStatePending, state)
}

func TestCheck_servingStaysServingWhileCnameHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.UmbrellaID)

	state, err := verifier(t, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
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

	state, err := verifier(t, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingClaimID)
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

	state, err := verifier(t, pointingAtEdge("track.serving.umbrella.test")).Check(ctx, s, fixtures.SendingDomainTrackingServingID)
	require.NoError(t, err)
	assert.Equal(t, sendingdomain.TrackingStateServing, state)
}
