package external_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getsphericon/sphericon/ent/suppression"
	"github.com/getsphericon/sphericon/ent/unsubscribe"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/eligibility"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eligible(t *testing.T, env *testhelper.TestEnv, dest, source string) eligibility.Decision {
	t.Helper()
	d, err := eligibility.Check(context.Background(), env.DB.Scoped(fixtures.AcmeID), eligibility.ChannelEmail, dest, source)
	require.NoError(t, err)
	return d
}

// Fixture contact 1 (alice) starts eligible everywhere; an agent-added Suppression
// takes her out of every send surface, transactional included.
func TestExternalSuppressionsCreateNarrowsEligibility(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	require.True(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts).Eligible)
	require.True(t, eligible(t, env, fixtures.ContactAliceEmail, "").Eligible)

	res, err := c.SuppressionsCreate(context.Background(), &externalapi.CreateSuppressionInput{Destination: "Alice@Example.com"})
	require.NoError(t, err)
	created, ok := res.(*externalapi.SuppressionResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, fixtures.ContactAliceEmail, created.Destination)
	assert.Equal(t, externalapi.SuppressionReasonManual, created.Reason)

	assert.Equal(t, eligibility.ReasonSuppressed, eligible(t, env, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts).Reason)
	assert.False(t, eligible(t, env, fixtures.ContactAliceEmail, "").Eligible, "suppression blocks transactional sends too")
}

// Fixture suppression SuppressionGhostBounce (a bounce) already exists: adding it
// again is idempotent and never rewrites the reason.
func TestExternalSuppressionsCreateIsIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	before, err := env.DB.Suppression.Query().Count(context.Background())
	require.NoError(t, err)

	res, err := c.SuppressionsCreate(context.Background(), &externalapi.CreateSuppressionInput{Destination: fixtures.SuppressionGhostBounceDestination})
	require.NoError(t, err)
	got, ok := res.(*externalapi.SuppressionResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, externalapi.SuppressionReasonBounce, got.Reason, "existing reason kept")

	after, err := env.DB.Suppression.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after)
	exists, err := env.DB.Suppression.Query().Where(suppression.DestinationEQ(fixtures.SuppressionGhostBounceDestination)).Exist(context.Background())
	require.NoError(t, err)
	assert.True(t, exists)
}

// An unsubscribe defaults to the broadcasts source: alice leaves broadcasts but
// remains eligible for automations and transactional mail.
func TestExternalUnsubscribesCreateDefaultsToBroadcasts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	require.True(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts).Eligible)

	res, err := c.UnsubscribesCreate(context.Background(), &externalapi.CreateUnsubscribeInput{Destination: "Alice@Example.com"})
	require.NoError(t, err)
	created, ok := res.(*externalapi.UnsubscribeResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, fixtures.ContactAliceEmail, created.Destination)
	assert.Equal(t, eligibility.SourceBroadcasts, created.SendingSource)

	assert.False(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts).Eligible)
	assert.True(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.AutomationSource(fixtures.AutomationWelcomeSeriesID)).Eligible)
	assert.True(t, eligible(t, env, fixtures.ContactAliceEmail, "").Eligible)
}

func TestExternalUnsubscribesCreateEverythingAndIdempotent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	in := &externalapi.CreateUnsubscribeInput{
		Destination:   fixtures.ContactAliceEmail,
		SendingSource: externalapi.NewOptString(eligibility.SourceEverything),
	}
	for range 2 {
		res, err := c.UnsubscribesCreate(context.Background(), in)
		require.NoError(t, err)
		require.IsType(t, &externalapi.UnsubscribeResource{}, res)
	}
	n, err := env.DB.Unsubscribe.Query().Where(unsubscribe.DestinationEQ(fixtures.ContactAliceEmail)).Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.False(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.AutomationSource(fixtures.AutomationWelcomeSeriesID)).Eligible)
}

func TestExternalUnsubscribesCreateAutomationSource(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	in := &externalapi.CreateUnsubscribeInput{
		Destination:   fixtures.ContactAliceEmail,
		SendingSource: externalapi.NewOptString(eligibility.AutomationSource(fixtures.AutomationWelcomeSeriesID)),
	}
	res, err := c.UnsubscribesCreate(context.Background(), in)
	require.NoError(t, err)
	require.IsType(t, &externalapi.UnsubscribeResource{}, res)
	assert.False(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.AutomationSource(fixtures.AutomationWelcomeSeriesID)).Eligible)
	assert.True(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts).Eligible)
}

func TestExternalUnsubscribesCreateRejectsUnknownSource(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:write")
	for _, src := range []string{"nonsense", "automation:999999", "automation:abc"} {
		res, err := c.UnsubscribesCreate(context.Background(), &externalapi.CreateUnsubscribeInput{
			Destination: fixtures.ContactAliceEmail, SendingSource: externalapi.NewOptString(src),
		})
		require.NoError(t, err)
		assert.IsTypef(t, &externalapi.UnsubscribesCreateUnprocessableEntity{}, res, src)
	}
}

func TestExternalConsentRequiresWriteScope(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "contacts:read")
	r1, err := c.SuppressionsCreate(context.Background(), &externalapi.CreateSuppressionInput{Destination: fixtures.ContactAliceEmail})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SuppressionsCreateForbidden{}, r1)
	r2, err := c.UnsubscribesCreate(context.Background(), &externalapi.CreateUnsubscribeInput{Destination: fixtures.ContactAliceEmail})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.UnsubscribesCreateForbidden{}, r2)
	assert.True(t, eligible(t, env, fixtures.ContactAliceEmail, eligibility.SourceBroadcasts).Eligible)
}

// Consent only narrows through /api (ADR 0016): the contract has no operation that
// resubscribes, removes an Unsubscribe or lifts a Suppression.
func TestExternalContractHasNoWideningConsentOperation(t *testing.T) {
	raw, err := os.ReadFile("../../../openapi/external.openapi.json")
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(raw, &spec))

	type route struct{ method, path, id string }
	var routes []route
	for path, methods := range spec.Paths {
		for method, op := range methods {
			routes = append(routes, route{method: method, path: path, id: strings.ToLower(op.OperationID + " " + path)})
		}
	}
	for _, r := range routes {
		assert.NotContains(t, r.id, "resubscribe", "%s %s", r.method, r.path)
	}
	consent := lo.Filter(routes, func(r route, _ int) bool {
		return strings.Contains(r.path, "suppression") || strings.Contains(r.path, "unsubscribe")
	})
	for _, r := range consent {
		assert.Equalf(t, "post", r.method, "%s %s: consent routes only narrow (create)", r.method, r.path)
	}
	assert.Len(t, consent, 2, "exactly add-suppression and unsubscribe")
}
