package trackingdomains_test

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

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackingdomain"
	"github.com/getsphericon/sphericon/ent/trackinghostchange"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/trackingdomains"
	"github.com/getsphericon/sphericon/internal/validation"
)

const (
	edge   = testhelper.TrackingCNAMETarget
	secret = "probe-secret"
)

var clock = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

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

// answering is the probe a healthy edge gives every host: 200 and the value for the host asked.
type answering struct{}

func (answering) Do(req *http.Request) (*http.Response, error) {
	body := trackingdomains.ProbeValue(secret, req.URL.Hostname())
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

var down = probeAnswer{err: errors.New("connection refused")}

// atEdge is a DNS zone in which every host given is a CNAME to the edge.
func atEdge(hosts ...string) map[string]mockdns.Zone {
	zones := map[string]mockdns.Zone{}
	for _, h := range hosts {
		zones[h+"."] = mockdns.Zone{CNAME: edge + "."}
	}
	return zones
}

// module builds the module over the system mailbox, the way the composition root does.
func module(t *testing.T, env *testhelper.TestEnv, zones map[string]mockdns.Zone, probe trackingdomains.Doer) *trackingdomains.Module {
	t.Helper()
	return trackingdomains.New(env.DB, env.Bus, accounts.OwnerNotifier(env.DB, env.SystemMail), trackingdomains.Config{
		Checker: &trackingdomains.Checker{
			Lookup: dnstest.Resolver(t, zones).LookupCNAME,
			Target: edge,
			HTTP:   probe,
			Secret: secret,
		},
		OwnHosts: []string{"app.sphericon.test", "t.sphericon.test"},
		Now:      func() time.Time { return clock },
	})
}

func get(t *testing.T, env *testhelper.TestEnv, id int64) *ent.TrackingDomain {
	t.Helper()
	row, err := env.DB.TrackingDomain.Get(t.Context(), id)
	require.NoError(t, err)
	return row
}

func gone(t *testing.T, env *testhelper.TestEnv, id int64) bool {
	t.Helper()
	_, err := env.DB.TrackingDomain.Get(t.Context(), id)
	if ent.IsNotFound(err) {
		return true
	}
	require.NoError(t, err)
	return false
}

func logged(t *testing.T, env *testhelper.TestEnv, host string) []trackinghostchange.State {
	t.Helper()
	rows, err := env.DB.TrackingHostChange.Query().Where(trackinghostchange.Host(host)).
		Order(ent.Asc(trackinghostchange.FieldID)).All(t.Context())
	require.NoError(t, err)
	out := make([]trackinghostchange.State, len(rows))
	for i, r := range rows {
		out[i] = r.State
	}
	return out
}

// --- the host a Workspace may set ---

func TestCanonicalHostAcceptsASubdomainOnly(t *testing.T) {
	for raw, want := range map[string]string{
		"T.Example.COM.":         "t.example.com",
		"go.example.co.uk":       "go.example.co.uk",
		"трек.пример.рф":         "xn--e1akqh.xn--e1afmkfd.xn--p1ai",
		"t.brightpath.localhost": "t.brightpath.localhost",
	} {
		got, ok := trackingdomains.CanonicalHost(raw)
		assert.True(t, ok, raw)
		assert.Equal(t, want, got, raw)
	}
	for _, raw := range []string{"example.com", "example.co.uk", "co.uk", "com", "", "10.0.0.1", "t.exa mple.com"} {
		_, ok := trackingdomains.CanonicalHost(raw)
		assert.False(t, ok, raw)
	}
}

func TestSetRefusesAnInvalidOrOwnHost(t *testing.T) {
	env := testhelper.Setup(t)
	m := module(t, env, nil, nil)
	s := env.DB.Scoped(fixtures.KramericaID)

	for host, want := range map[string]error{
		"example.com":        trackingdomains.ErrInvalidHost,
		"app.sphericon.test": trackingdomains.ErrOwnHost,
		"t.sphericon.test":   trackingdomains.ErrOwnHost,
		edge:                 trackingdomains.ErrOwnHost,
	} {
		_, err := m.Set(t.Context(), s, host)
		require.ErrorIs(t, err, want, host)
		var refusal *validation.RefusalError
		require.ErrorAs(t, err, &refusal, host)
	}
}

func TestSetIsRefusedWhileTheFeatureIsOff(t *testing.T) {
	env := testhelper.Setup(t)
	m := trackingdomains.New(env.DB, env.Bus, nil, trackingdomains.Config{})

	_, err := m.Set(t.Context(), env.DB.Scoped(fixtures.KramericaID), "t.kramerica.test")
	require.ErrorIs(t, err, trackingdomains.ErrOff)
	assert.Empty(t, m.Target())
}

// --- set, change and remove ---

func TestSetWithNoTrackingDomainMakesItCurrentAndPending(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, nil, nil).Set(t.Context(), env.DB.Scoped(fixtures.KramericaID), "T.Kramerica.TEST")
	require.NoError(t, err)
	assert.Equal(t, "t.kramerica.test", row.Host)
	assert.Equal(t, trackingdomain.RoleCurrent, row.Role)
	assert.Equal(t, trackingdomain.StatePending, row.State)
}

func TestSettingAHostTheWorkspaceUsesChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, nil, nil).Set(t.Context(), env.DB.Scoped(fixtures.UmbrellaID), fixtures.TrackingDomainServingHost)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.TrackingDomainServingID, row.ID)
	assert.Equal(t, trackingdomain.RoleReplacement, get(t, env, fixtures.TrackingDomainReplacementID).Role)
}

// While the current host serves, a new one waits as the replacement and the pending one it
// replaces is discarded: one replacement at a time.
func TestSetWhileCurrentServesMakesTheReplacementAndDiscardsThePendingOne(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, nil, nil).Set(t.Context(), env.DB.Scoped(fixtures.UmbrellaID), "links.umbrella.test")
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.RoleReplacement, row.Role)
	assert.Equal(t, trackingdomain.StatePending, row.State)
	assert.True(t, gone(t, env, fixtures.TrackingDomainReplacementID))
	assert.Equal(t, trackingdomain.RoleCurrent, get(t, env, fixtures.TrackingDomainServingID).Role)
}

// A current host that does not serve keeps no mail on it: the new one takes its place at once,
// and the old one, which never served, is deleted.
func TestSetWhileCurrentDoesNotServeReplacesItAtOnce(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, nil, nil).Set(t.Context(), env.DB.Scoped(fixtures.InitechID), "links.initech.test")
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.RoleCurrent, row.Role)
	assert.True(t, gone(t, env, fixtures.TrackingDomainDNSVerifiedID))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved}, logged(t, env, fixtures.TrackingDomainDNSVerifiedHost))
}

// A retired host set again comes back, and one that still serves takes over at once: the
// current one is retired in its place and the Owners are told.
func TestSettingARetiredHostThatServesBringsItBackAtOnce(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, nil, nil).Set(t.Context(), env.DB.Scoped(fixtures.UmbrellaID), fixtures.TrackingDomainRetiredHost)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.TrackingDomainRetiredID, row.ID)
	assert.Equal(t, trackingdomain.RoleCurrent, row.Role)
	assert.Equal(t, trackingdomain.RoleRetired, get(t, env, fixtures.TrackingDomainServingID).Role)
	assert.True(t, gone(t, env, fixtures.TrackingDomainReplacementID))

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Subject, fixtures.TrackingDomainRetiredHost)
}

// Removing a host that served retires it, so the links sent on it keep working, and the
// replacement becomes current.
func TestRemovingTheServingCurrentRetiresItAndPromotesTheReplacement(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.UmbrellaID)

	require.NoError(t, module(t, env, nil, nil).Remove(t.Context(), s, fixtures.TrackingDomainServingID))

	assert.Equal(t, trackingdomain.RoleRetired, get(t, env, fixtures.TrackingDomainServingID).Role)
	assert.Equal(t, trackingdomain.RoleCurrent, get(t, env, fixtures.TrackingDomainReplacementID).Role)
	assert.Empty(t, logged(t, env, fixtures.TrackingDomainServingHost), "a retired host is still served")
	host, err := module(t, env, nil, nil).ServingHost(t.Context(), s)
	require.NoError(t, err)
	assert.Empty(t, host, "new mail uses the Instance's host until the replacement serves")
}

func TestRemovingAHostThatNeverServedDeletesIt(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, module(t, env, nil, nil).Remove(t.Context(), env.DB.Scoped(fixtures.InitechID), fixtures.TrackingDomainDNSVerifiedID))
	assert.True(t, gone(t, env, fixtures.TrackingDomainDNSVerifiedID))
}

func TestRemovingAReplacementOrARetiredHostDeletesIt(t *testing.T) {
	env := testhelper.Setup(t)
	m := module(t, env, nil, nil)
	s := env.DB.Scoped(fixtures.UmbrellaID)

	require.NoError(t, m.Remove(t.Context(), s, fixtures.TrackingDomainReplacementID))
	require.NoError(t, m.Remove(t.Context(), s, fixtures.TrackingDomainRetiredID))

	assert.True(t, gone(t, env, fixtures.TrackingDomainReplacementID))
	assert.True(t, gone(t, env, fixtures.TrackingDomainRetiredID))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved}, logged(t, env, fixtures.TrackingDomainRetiredHost))
	assert.Equal(t, trackingdomain.RoleCurrent, get(t, env, fixtures.TrackingDomainServingID).Role)
}

func TestRemovingAnotherWorkspacesHostIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)

	err := module(t, env, nil, nil).Remove(t.Context(), env.DB.Scoped(fixtures.WayneID), fixtures.TrackingDomainServingID)
	require.ErrorIs(t, err, trackingdomains.ErrNotFound)
}

func TestListPutsCurrentThenReplacementThenRetired(t *testing.T) {
	env := testhelper.Setup(t)

	rows, err := module(t, env, nil, nil).List(t.Context(), env.DB.Scoped(fixtures.UmbrellaID))
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, []trackingdomain.Role{trackingdomain.RoleCurrent, trackingdomain.RoleReplacement, trackingdomain.RoleRetired},
		[]trackingdomain.Role{rows[0].Role, rows[1].Role, rows[2].Role})
}

func TestServingHostIsTheCurrentHostOnlyWhileItServes(t *testing.T) {
	env := testhelper.Setup(t)
	m := module(t, env, nil, nil)

	host, err := m.ServingHost(t.Context(), env.DB.Scoped(fixtures.UmbrellaID))
	require.NoError(t, err)
	assert.Equal(t, fixtures.TrackingDomainServingHost, host)

	host, err = m.ServingHost(t.Context(), env.DB.Scoped(fixtures.InitechID))
	require.NoError(t, err)
	assert.Empty(t, host, "dns_verified does not serve")
}

// --- the check ---

func TestVerifyFindsTheCnameAndStopsAtDNSVerifiedWithoutAProbe(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)

	row, err := module(t, env, atEdge(fixtures.TrackingDomainAcmeHost), down).Verify(t.Context(), s, fixtures.TrackingDomainAcmeID)
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.StateDNSVerified, row.State)
	assert.True(t, clock.Equal(*row.CheckedAt))
	assert.True(t, clock.Equal(*row.StateChangedAt))
	assert.Nil(t, row.ServedAt)
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateDNSVerified}, logged(t, env, fixtures.TrackingDomainAcmeHost))
}

func TestVerifyWithoutTheCnameStaysPending(t *testing.T) {
	env := testhelper.Setup(t)

	for name, zones := range map[string]map[string]mockdns.Zone{
		"missing": nil,
		"wrong":   {fixtures.TrackingDomainAcmeHost + ".": {CNAME: "elsewhere.example.net."}},
	} {
		t.Run(name, func(t *testing.T) {
			row, err := module(t, env, zones, answering{}).Verify(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.TrackingDomainAcmeID)
			require.NoError(t, err)
			assert.Equal(t, trackingdomain.StatePending, row.State)
			assert.True(t, clock.Equal(*row.CheckedAt))
			assert.Nil(t, row.StateChangedAt)
		})
	}
}

func TestAnAnsweringProbeMakesTheHostServingAndPublishesTheChange(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, atEdge(fixtures.TrackingDomainDNSVerifiedHost), answering{}).
		Verify(t.Context(), env.DB.Scoped(fixtures.InitechID), fixtures.TrackingDomainDNSVerifiedID)
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.StateServing, row.State)
	assert.True(t, clock.Equal(*row.ServedAt))

	changes := env.OutboxEvents(t, events.NameTrackingDomainChanged)
	require.Len(t, changes, 1)
	change := changes[0].(*events.TrackingDomainChanged)
	assert.Equal(t, fixtures.TrackingDomainDNSVerifiedHost, change.Host)
	assert.EqualValues(t, fixtures.TrackingDomainDNSVerifiedID, change.TrackingDomainID)
	assert.Equal(t, "dns_verified", change.From)
	assert.Equal(t, "serving", change.To)
	assert.Empty(t, env.SystemMail.Messages(), "a promotion of the current host tells nobody")
}

func TestNoSuccessfulProbeNeverServes(t *testing.T) {
	cases := map[string]probeAnswer{
		"unreachable":  {err: errors.New("tls handshake failure")},
		"not ok":       {status: http.StatusNotFound, body: "nope"},
		"foreign body": {status: http.StatusOK, body: "parked"},
		"other host":   {status: http.StatusOK, body: trackingdomains.ProbeValue(secret, "t.other.test")},
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)

			row, err := module(t, env, atEdge(fixtures.TrackingDomainDNSVerifiedHost), answer).
				Verify(t.Context(), env.DB.Scoped(fixtures.InitechID), fixtures.TrackingDomainDNSVerifiedID)
			require.NoError(t, err)
			assert.Equal(t, trackingdomain.StateDNSVerified, row.State)
			assert.Empty(t, env.OutboxEvents(t, events.NameTrackingDomainChanged))
		})
	}
}

// The current host that stops answering falls back a step and the Owners are told.
func TestAServingCurrentHostThatStopsAnsweringIsDemotedAndTheOwnersAreTold(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, atEdge(fixtures.TrackingDomainServingHost), down).
		Verify(t.Context(), env.DB.Scoped(fixtures.UmbrellaID), fixtures.TrackingDomainServingID)
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.StateDNSVerified, row.State)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, fixtures.UmbrellaOwnerRitaEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Subject, fixtures.TrackingDomainServingHost)
}

func TestAResolverFailureChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	m := trackingdomains.New(env.DB, env.Bus, nil, trackingdomains.Config{
		Checker: &trackingdomains.Checker{
			Lookup: func(context.Context, string) (string, error) { return "", errors.New("servfail") },
			Target: edge,
		},
		Now: func() time.Time { return clock },
	})

	_, err := m.Verify(t.Context(), env.DB.Scoped(fixtures.InitechID), fixtures.TrackingDomainDNSVerifiedID)
	require.Error(t, err)
	row := get(t, env, fixtures.TrackingDomainDNSVerifiedID)
	assert.Equal(t, trackingdomain.StateDNSVerified, row.State)
	assert.False(t, clock.Equal(*row.CheckedAt))
}

// Umbrella holds t.umbrella.test; Wayne's claim on the same host cannot verify while Umbrella
// is verified, and says why.
func TestASecondWorkspaceCannotVerifyAHeldHost(t *testing.T) {
	env := testhelper.Setup(t)

	row, err := module(t, env, atEdge(fixtures.TrackingDomainClaimHost), answering{}).
		Verify(t.Context(), env.DB.Scoped(fixtures.WayneID), fixtures.TrackingDomainClaimID)
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.StatePending, row.State)
	require.NotNil(t, row.Reason)
	assert.Equal(t, trackingdomain.ReasonHostTaken, *row.Reason)
	assert.Nil(t, get(t, env, fixtures.TrackingDomainServingID).Reason)

	// The reason goes with the next check that does not lose the host.
	row, err = module(t, env, nil, nil).Verify(t.Context(), env.DB.Scoped(fixtures.WayneID), fixtures.TrackingDomainClaimID)
	require.NoError(t, err)
	assert.Nil(t, row.Reason)
}

// A replacement that reaches serving takes over: the current host is retired, and the Owners
// are told to keep its CNAME while its links matter.
func TestAReplacementThatServesTakesOverAndRetiresTheCurrentHost(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.UmbrellaID)
	m := module(t, env, atEdge(fixtures.TrackingDomainReplacementHost), answering{})

	row, err := m.Verify(t.Context(), s, fixtures.TrackingDomainReplacementID)
	require.NoError(t, err)
	assert.Equal(t, trackingdomain.RoleCurrent, row.Role)
	assert.Equal(t, trackingdomain.StateServing, row.State)
	assert.Equal(t, trackingdomain.RoleRetired, get(t, env, fixtures.TrackingDomainServingID).Role)

	host, err := m.ServingHost(t.Context(), s)
	require.NoError(t, err)
	assert.Equal(t, fixtures.TrackingDomainReplacementHost, host)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Subject, fixtures.TrackingDomainReplacementHost)
	assert.Contains(t, msgs[0].Text, fixtures.TrackingDomainServingHost)
}

// A retired host stays while its CNAME reaches the edge, whatever the probe says.
func TestARetiredHostStaysWhileItsCnameReachesTheEdge(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, module(t, env, atEdge(fixtures.TrackingDomainRetiredHost), down).Recheck(t.Context(), fixtures.TrackingDomainRetiredID))

	row := get(t, env, fixtures.TrackingDomainRetiredID)
	assert.Equal(t, trackingdomain.RoleRetired, row.Role)
	assert.Equal(t, trackingdomain.StateServing, row.State)
	assert.True(t, clock.Equal(*row.CheckedAt))
	assert.Empty(t, env.SystemMail.Messages())
}

// A retired host whose CNAME left goes: the edge tears it down and the Owners are told.
func TestARetiredHostWhoseCnameLeftGoes(t *testing.T) {
	env := testhelper.Setup(t)

	require.NoError(t, module(t, env, nil, answering{}).Recheck(t.Context(), fixtures.TrackingDomainRetiredID))

	assert.True(t, gone(t, env, fixtures.TrackingDomainRetiredID))
	assert.Equal(t, []trackinghostchange.State{trackinghostchange.StateRemoved}, logged(t, env, fixtures.TrackingDomainRetiredHost))
	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Subject, fixtures.TrackingDomainRetiredHost)
}

func TestDueIsEveryVerifiedHostInAnyRole(t *testing.T) {
	env := testhelper.Setup(t)

	ids, err := module(t, env, nil, nil).Due(t.Context())
	require.NoError(t, err)
	assert.Contains(t, ids, int64(fixtures.TrackingDomainServingID))
	assert.Contains(t, ids, int64(fixtures.TrackingDomainDNSVerifiedID))
	assert.Contains(t, ids, int64(fixtures.TrackingDomainRetiredID))
	assert.NotContains(t, ids, int64(fixtures.TrackingDomainReplacementID))
	assert.NotContains(t, ids, int64(fixtures.TrackingDomainClaimID))
}

// The lifecycle lists six edges: each of the three states moves to both others, and the check
// takes every one of them from what DNS and the probe answer.
func TestEveryEdgeOfTheStateLifecycle(t *testing.T) {
	const (
		pending  = trackingdomain.StatePending
		verified = trackingdomain.StateDNSVerified
		serving  = trackingdomain.StateServing
	)
	type row struct {
		ws   int64
		id   int64
		host string
	}
	acme := row{fixtures.AcmeID, fixtures.TrackingDomainAcmeID, fixtures.TrackingDomainAcmeHost}
	initech := row{fixtures.InitechID, fixtures.TrackingDomainDNSVerifiedID, fixtures.TrackingDomainDNSVerifiedHost}
	umbrella := row{fixtures.UmbrellaID, fixtures.TrackingDomainServingID, fixtures.TrackingDomainServingHost}
	cases := []struct {
		name     string
		row      row
		zones    map[string]mockdns.Zone
		probe    trackingdomains.Doer
		from, to trackingdomain.State
	}{
		{"pending to dns_verified", acme, atEdge(acme.host), down, pending, verified},
		{"pending to serving", acme, atEdge(acme.host), answering{}, pending, serving},
		{"dns_verified to pending", initech, nil, down, verified, pending},
		{"dns_verified to serving", initech, atEdge(initech.host), answering{}, verified, serving},
		{"serving to pending", umbrella, nil, down, serving, pending},
		{"serving to dns_verified", umbrella, atEdge(umbrella.host), down, serving, verified},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testhelper.Setup(t)

			got, err := module(t, env, c.zones, c.probe).Verify(t.Context(), env.DB.Scoped(c.row.ws), c.row.id)
			require.NoError(t, err)
			assert.Equal(t, c.to, got.State)
			assert.True(t, clock.Equal(*got.StateChangedAt))

			changes := env.OutboxEvents(t, events.NameTrackingDomainChanged)
			require.Len(t, changes, 1)
			assert.Equal(t, string(c.from), changes[0].(*events.TrackingDomainChanged).From)
			assert.Equal(t, string(c.to), changes[0].(*events.TrackingDomainChanged).To)
		})
	}
}
