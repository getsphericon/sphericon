package operator_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/operator"
	"github.com/getsphericon/sphericon/ent/auditentry"
	entcredential "github.com/getsphericon/sphericon/ent/servicecredential"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var (
	primary  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, fixtures.ServiceCredentialPrimarySecret)
	second   = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialSecondPrefix, fixtures.ServiceCredentialSecondSecret)
	readOnly = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialReadOnlyPrefix, fixtures.ServiceCredentialReadOnlySecret)
	revoked  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialRevokedPrefix, fixtures.ServiceCredentialRevokedSecret)
	expired  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialExpiredPrefix, fixtures.ServiceCredentialExpiredSecret)
)

// identity reads the credential's own identity, failing unless it is answered.
func identity(t *testing.T, c *serviceapi.Client) *serviceapi.ServiceIdentityResource {
	t.Helper()
	res, err := c.ServiceIdentityGet(t.Context())
	require.NoError(t, err)
	id, ok := res.(*serviceapi.ServiceIdentityResource)
	require.Truef(t, ok, "the identity is answered, got %T", res)
	return id
}

// getIdentity is a raw GET of the identity path on handler, with an optional bearer.
func getIdentity(t *testing.T, handler http.Handler, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/identity", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestAServiceReadsItsOwnIdentityOnTheInternalListener(t *testing.T) {
	h := newHarness(t)

	got := identity(t, h.env.ServiceClient(t, primary))

	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, got.Name)
	assert.ElementsMatch(t, []serviceapi.ServiceScope{
		serviceapi.ServiceScopeUsageRead, serviceapi.ServiceScopeWorkspacesRead, serviceapi.ServiceScopeHoldsWrite,
	}, got.Scopes)
	assert.True(t, got.ExpiresAt.IsNull() || !got.ExpiresAt.IsSet(), "a credential without an expiry reports none")

	withExpiry := identity(t, h.env.ServiceClient(t, second))
	expiry, ok := withExpiry.ExpiresAt.Get()
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), time.Time(expiry), time.Hour)
}

func TestTheServiceEndpointsAreServedOverRealHTTPOnTheInternalHandler(t *testing.T) {
	h := newHarness(t)
	srv := httptest.NewServer(h.env.Internal)
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/service/identity", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+primary)
	res, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, body["name"])
}

func TestTheServiceEndpointsHaveNoRouteOnThePublicPort(t *testing.T) {
	h := newHarness(t)

	rec := getIdentity(t, h.env.Server, primary)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Body.String(), fixtures.ServiceCredentialPrimaryName)
}

func TestTheInternalListenerServesNothingButTheServiceEndpoints(t *testing.T) {
	h := newHarness(t)

	for _, path := range []string{"/", "/site/workspaces", "/operator/me", "/api/contacts", "/metrics", "/healthz"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.env.Internal.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}

func TestAnUnknownMalformedRevokedOrExpiredCredentialIsRefused(t *testing.T) {
	h := newHarness(t)
	wrongSecret := testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, "not-the-secret")
	anAPIToken := "omtk_" + fixtures.AnchorTokenPrefix + "_" + fixtures.AnchorTokenSecret

	for name, bearer := range map[string]string{
		"none":          "",
		"malformed":     "garbage",
		"unknown":       testhelper.ServiceCredentialValue("nosuchprefix", "whatever"),
		"wrong secret":  wrongSecret,
		"revoked":       revoked,
		"expired":       expired,
		"an api token":  anAPIToken,
		"the site JWT":  "eyJhbGciOiJIUzI1NiJ9.e30.x",
		"empty secret":  testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, ""),
		"upper-cased":   strings.ToUpper(primary),
		"extra segment": primary + "_x",
	} {
		t.Run(name, func(t *testing.T) {
			rec := getIdentity(t, h.env.Internal, bearer)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
		})
	}
}

func TestACredentialExpiresWhenTheClockPassesItsExpiry(t *testing.T) {
	h := newHarness(t)
	in := time.Hour
	issued, err := h.env.Operators.IssueServiceCredential(t.Context(), operator.IssueInput{
		Name: "service-c", Scopes: []operator.Scope{operator.ScopeUsageRead}, ExpiresAt: new(h.clock.now().Add(in)),
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, issued.Value).Code)

	h.clock.t = h.clock.t.Add(in + time.Second)
	assert.Equal(t, http.StatusUnauthorized, getIdentity(t, h.env.Internal, issued.Value).Code)
}

func TestTwoActiveCredentialsOfOneServiceWorkAtOnce(t *testing.T) {
	h := newHarness(t)

	a := identity(t, h.env.ServiceClient(t, primary))
	b := identity(t, h.env.ServiceClient(t, second))

	assert.Equal(t, a.Name, b.Name)
}

func TestIssuingShowsTheValueOnceAndStoresOnlyAHash(t *testing.T) {
	h := newHarness(t)

	issued, err := h.env.Operators.IssueServiceCredential(t.Context(), operator.IssueInput{
		Name: " service-c ", Scopes: []operator.Scope{operator.ScopeHoldsWrite, operator.ScopeUsageRead, operator.ScopeUsageRead},
	})
	require.NoError(t, err)

	assert.Equal(t, "service-c", issued.Name)
	assert.Equal(t, []operator.Scope{operator.ScopeHoldsWrite, operator.ScopeUsageRead}, issued.Scopes, "sorted, without duplicates")
	assert.True(t, strings.HasPrefix(issued.Value, "omsc_"))
	row := h.env.DB.ServiceCredential.GetX(t.Context(), issued.ID)
	assert.NotContains(t, issued.Value, row.SecretHash)
	parsed := credentials.ParseServiceCredential(issued.Value)
	require.NotNil(t, parsed)
	assert.NotContains(t, row.SecretHash, parsed.Secret, "only a hash of the secret is stored")
	assert.Equal(t, "service-c", identity(t, h.env.ServiceClient(t, issued.Value)).Name, "the value works")
}

func TestIssuingRefusesABadInput(t *testing.T) {
	h := newHarness(t)
	past := h.clock.now().Add(-time.Minute)

	for name, tc := range map[string]struct {
		in   operator.IssueInput
		want error
	}{
		"no name":       {operator.IssueInput{Name: " ", Scopes: []operator.Scope{operator.ScopeUsageRead}}, operator.ErrInvalidCredential},
		"no scopes":     {operator.IssueInput{Name: "s"}, operator.ErrInvalidScope},
		"unknown scope": {operator.IssueInput{Name: "s", Scopes: []operator.Scope{"contacts:read"}}, operator.ErrInvalidScope},
		"past expiry":   {operator.IssueInput{Name: "s", Scopes: []operator.Scope{operator.ScopeUsageRead}, ExpiresAt: &past}, operator.ErrInvalidCredential},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.env.Operators.IssueServiceCredential(t.Context(), tc.in)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestRotatingIssuesASecondCredentialAndLeavesTheFirstActive(t *testing.T) {
	h := newHarness(t)

	rotated, err := h.env.Operators.RotateServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID, nil)
	require.NoError(t, err)

	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, rotated.Name)
	assert.Len(t, rotated.Scopes, 3, "the scopes carry over")
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, rotated.Value).Code)
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code, "no downtime: the old value still works until it is revoked")
}

func TestRotatingARevokedOrExpiredOrMissingCredentialIsRefused(t *testing.T) {
	h := newHarness(t)

	for _, id := range []int{fixtures.ServiceCredentialRevokedID, fixtures.ServiceCredentialExpiredID} {
		_, err := h.env.Operators.RotateServiceCredential(t.Context(), int64(id), nil)
		require.ErrorIs(t, err, operator.ErrCredentialInactive)
	}
	_, err := h.env.Operators.RotateServiceCredential(t.Context(), 999999, nil)
	require.ErrorIs(t, err, operator.ErrCredentialNotFound)
}

func TestARevokedCredentialIsRefusedAtOnceAndRevokingTwiceChangesNothing(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code)

	require.NoError(t, h.env.Operators.RevokeServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID))
	firstRevoked := h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialPrimaryID).RevokedAt
	h.clock.t = h.clock.t.Add(time.Hour)
	require.NoError(t, h.env.Operators.RevokeServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID))

	assert.Equal(t, http.StatusUnauthorized, getIdentity(t, h.env.Internal, primary).Code)
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, second).Code, "its sibling is untouched")
	assert.Equal(t, firstRevoked, h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialPrimaryID).RevokedAt)
	require.ErrorIs(t, h.env.Operators.RevokeServiceCredential(t.Context(), 999999), operator.ErrCredentialNotFound)
}

func TestTheListShowsEachCredentialsStateAndWhenItWasLastUsed(t *testing.T) {
	h := newHarness(t)
	before, err := h.env.Operators.ServiceCredentials(t.Context(), fixtures.ServiceCredentialPrimaryName)
	require.NoError(t, err)
	require.NotEmpty(t, before)
	for _, c := range before {
		assert.Nil(t, c.LastUsedAt, "never used")
	}

	require.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code)

	listed, err := h.env.Operators.ServiceCredentials(t.Context(), fixtures.ServiceCredentialPrimaryName)
	require.NoError(t, err)
	byID := map[int64]time.Time{}
	for _, c := range listed {
		assert.Equal(t, fixtures.ServiceCredentialPrimaryName, c.Name)
		if c.LastUsedAt != nil {
			byID[c.ID] = *c.LastUsedAt
		}
	}
	require.Len(t, byID, 1, "only the credential that was used")
	assert.WithinDuration(t, h.clock.now(), byID[fixtures.ServiceCredentialPrimaryID], time.Millisecond, "at the clock's time")

	all, err := h.env.Operators.ServiceCredentials(t.Context(), "")
	require.NoError(t, err)
	assert.Greater(t, len(all), len(listed), "no name lists every service's")
}

func TestACredentialCannotActOutsideItsScopes(t *testing.T) {
	h := newHarness(t)
	needsHolds := map[string][]operator.Scope{"NeedsHolds": {operator.ScopeHoldsWrite}, "NeedsNothing": nil}
	security := operator.NewServiceSecurity(h.env.Operators, needsHolds)

	_, err := security.HandleBearerAuth(t.Context(), "NeedsHolds", serviceapi.BearerAuth{Token: readOnly})
	require.ErrorIs(t, err, operator.ErrForbidden, "a valid credential without the scope")

	ctx, err := security.HandleBearerAuth(t.Context(), "NeedsHolds", serviceapi.BearerAuth{Token: primary})
	require.NoError(t, err)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, operator.CurrentService(ctx).Name)

	_, err = security.HandleBearerAuth(t.Context(), "NeedsNothing", serviceapi.BearerAuth{Token: readOnly})
	require.NoError(t, err, "an operation that needs no scope is open to any valid credential")

	_, err = security.HandleBearerAuth(t.Context(), "NotListed", serviceapi.BearerAuth{Token: primary})
	require.ErrorIs(t, err, operator.ErrForbidden, "an operation with no entry is refused to every credential")
}

func TestEveryServiceOperationDeclaresItsScopes(t *testing.T) {
	for _, op := range operator.ServiceOperations() {
		_, declared := operator.ServiceOperationScopes[op]
		assert.True(t, declared, "%s must have an entry in ServiceOperationScopes, even an empty one", op)
	}
}

func TestNoEndpointMintsOrChangesACredential(t *testing.T) {
	surfaces := map[string][]string{
		"service":  operator.ServiceOperations(),
		"operator": methodNames(reflect.TypeFor[operatorapi.Handler]()),
	}
	for surface, ops := range surfaces {
		require.NotEmpty(t, ops, surface)
		for _, op := range ops {
			for _, banned := range []string{"Credential", "Secret", "Issue", "Rotate", "Revoke"} {
				assert.NotContains(t, op, banned, "the %s surface never mints or changes a credential", surface)
			}
		}
	}
}

func methodNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumMethod())
	for i := range t.NumMethod() {
		names = append(names, t.Method(i).Name)
	}
	return names
}

func TestTheServiceEndpointsAreRateLimitedPerCredential(t *testing.T) {
	h := newHarness(t, testhelper.WithRateLimits(config.RateLimits{ServicePerMinute: 2}))

	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code)
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code)
	limited := getIdentity(t, h.env.Internal, primary)
	assert.Equal(t, http.StatusTooManyRequests, limited.Code)
	assert.NotEmpty(t, limited.Header().Get("Retry-After"))

	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, readOnly).Code, "each credential draws a budget of its own")
}

func TestARunOfFailedCredentialsBlocksTheClientAddress(t *testing.T) {
	h := newHarness(t, testhelper.WithRateLimits(config.RateLimits{FailedAuth: 2}))

	assert.Equal(t, http.StatusUnauthorized, getIdentity(t, h.env.Internal, "garbage").Code)
	assert.Equal(t, http.StatusUnauthorized, getIdentity(t, h.env.Internal, "garbage").Code)
	assert.Equal(t, http.StatusTooManyRequests, getIdentity(t, h.env.Internal, "garbage").Code)
	assert.Equal(t, http.StatusTooManyRequests, getIdentity(t, h.env.Internal, primary).Code, "even a good credential waits out the block")
}

// A write made under a credential is attributed to its name in the Audit log, and the
// Workspace's own team reads it as "sphericon staff".
func TestAWriteUnderACredentialIsAttributedToItsNameInTheAuditLog(t *testing.T) {
	h := newHarness(t)
	cred := h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialPrimaryID)
	scoped := h.env.Bus.Act(h.env.DB.Scoped(fixtures.AcmeID), operator.ServiceActor(cred))
	_, err := scoped.Tag().Create().SetName("attributed-tag").Save(t.Context())
	require.NoError(t, err)
	h.env.DeliverToEE(t)

	stored := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().Where(auditentry.TargetName("attributed-tag")).OnlyX(t.Context())
	assert.Equal(t, events.ActorService, stored.ActorKind)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, *stored.ActorName)
	assert.Equal(t, strconv.Itoa(fixtures.ServiceCredentialPrimaryID), *stored.ActorID)

	console, _ := h.operatorSession(t)
	page := h.auditPage(t, console, fixtures.AcmeID, operatorapi.OperatorWorkspaceAuditListParams{})
	var shown *operatorapi.OperatorAuditEntryResource
	for _, e := range page.Items {
		if e.Target.Name.Value == "attributed-tag" {
			shown = &e
		}
	}
	require.NotNil(t, shown, "the Operator console lists the entry")
	assert.Equal(t, operatorapi.OperatorAuditActorKindService, shown.Actor.Kind)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, shown.Actor.Name.Value)

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	customerPage, ok := res.(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "got %T", res)
	for _, e := range customerPage.Items {
		if e.Target.Name.Value != "attributed-tag" {
			continue
		}
		assert.Equal(t, siteapi.SiteAuditActorKindOperator, e.Actor.Kind)
		assert.Equal(t, events.OperatorLabel, e.Actor.Name.Value)
		assert.False(t, e.Actor.ID.IsSet() && e.Actor.ID.Value != "")
	}
}

func TestWithoutALicenseNothingIsServedAndTheModuleRefuses(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	assert.Nil(t, env.Internal, "no handler for a listener to serve")
	_, err := env.Operators.IssueServiceCredential(t.Context(), operator.IssueInput{Name: "s", Scopes: []operator.Scope{operator.ScopeUsageRead}})
	require.ErrorIs(t, err, operator.ErrNotLicensed)
	_, err = env.Operators.RotateServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID, nil)
	require.ErrorIs(t, err, operator.ErrNotLicensed)
	require.ErrorIs(t, env.Operators.RevokeServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID), operator.ErrNotLicensed)
	_, err = env.Operators.ServiceCredentials(t.Context(), "")
	require.ErrorIs(t, err, operator.ErrNotLicensed)
	_, err = env.Operators.AuthenticateService(t.Context(), primary)
	require.ErrorIs(t, err, operator.ErrUnauthorized, "a valid value is refused without the license")
	assert.Equal(t, http.StatusNotFound, getIdentity(t, env.Server, primary).Code)
}

func TestTheStoredCredentialsHoldNoClearValue(t *testing.T) {
	h := newHarness(t)

	n := h.env.DB.ServiceCredential.Query().Where(entcredential.SecretHashContains(fixtures.ServiceCredentialPrimarySecret)).CountX(t.Context())

	assert.Zero(t, n)
}
