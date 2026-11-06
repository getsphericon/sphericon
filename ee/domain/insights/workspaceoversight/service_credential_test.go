package workspaceoversight_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	"github.com/getsphericon/sphericon/ent/auditentry"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var (
	primary  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, fixtures.ServiceCredentialPrimarySecret)
	second   = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialSecondPrefix, fixtures.ServiceCredentialSecondSecret)
	readOnly = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialReadOnlyPrefix, fixtures.ServiceCredentialReadOnlySecret)
	revoked  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialRevokedPrefix, fixtures.ServiceCredentialRevokedSecret)
	expired  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialExpiredPrefix, fixtures.ServiceCredentialExpiredSecret)
	// usersOnly, domainsOnly and holdsOnly each carry one scope only.
	usersOnly   = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialUsersReaderPrefix, fixtures.ServiceCredentialUsersReaderSecret)
	domainsOnly = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialDomainsReaderPrefix, fixtures.ServiceCredentialDomainsReaderSecret)
	holdsOnly   = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialHoldsOnlyPrefix, fixtures.ServiceCredentialHoldsOnlySecret)
)

// identity reads the credential's own identity, failing unless it is answered.
func identity(t *testing.T, c *serviceapi.Client) *serviceapi.ServiceIdentityResource {
	t.Helper()
	res, err := c.ServiceIdentityGet(t.Context())
	require.NoError(t, err)
	id := res
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

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/service/identity", http.NoBody)
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

	for _, path := range []string{"/", "/site/workspaces", "/staff/me", "/api/contacts", "/metrics", "/healthz"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		h.env.Internal.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}

func TestAnUnknownMalformedRevokedOrExpiredCredentialIsRefused(t *testing.T) {
	h := newHarness(t)
	wrongSecret := testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, fixtures.ServiceCredentialSecondSecret)
	anAPIToken := "omtk_" + fixtures.AnchorTokenPrefix + "_" + fixtures.AnchorTokenSecret

	for name, bearer := range map[string]string{
		"none":          "",
		"malformed":     "garbage",
		"unknown":       testhelper.ServiceCredentialValue("nosuchprefix", fixtures.ServiceCredentialPrimarySecret),
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
	expiresAt := h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialSecondID).ExpiresAt
	require.NotNil(t, expiresAt)

	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, second).Code)

	h.clock.t = expiresAt.Add(time.Second)
	assert.Equal(t, http.StatusUnauthorized, getIdentity(t, h.env.Internal, second).Code)
}

func TestTwoActiveCredentialsOfOneServiceWorkAtOnce(t *testing.T) {
	h := newHarness(t)

	a := identity(t, h.env.ServiceClient(t, primary))
	b := identity(t, h.env.ServiceClient(t, second))

	assert.Equal(t, a.Name, b.Name)
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
// Workspace's own team reads it as "Sphericon staff".
func TestAWriteUnderACredentialIsAttributedToItsNameInTheAuditLog(t *testing.T) {
	h := newHarness(t)
	cred := h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialPrimaryID)
	scoped := h.env.Bus.Act(h.env.DB.Scoped(fixtures.AcmeID), servicecredentials.ServiceActor(cred))
	// test writes: audit attribution of a scoped write is the subject
	_, err := scoped.Tag().Create().SetName("attributed-tag").Save(t.Context())
	require.NoError(t, err)
	h.env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	stored := h.env.DB.Scoped(fixtures.AcmeID).AuditEntry().Query().Where(auditentry.TargetName("attributed-tag")).OnlyX(t.Context())
	assert.Equal(t, events.ActorService, stored.ActorKind)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, *stored.ActorName)
	assert.Equal(t, strconv.Itoa(fixtures.ServiceCredentialPrimaryID), *stored.ActorID)

	console, _ := h.staffSession(t)
	page := h.auditPage(t, console, fixtures.AcmePublicID, staffeeapi.StaffWorkspaceAuditListParams{})
	shown, listed := lo.Find(page.Items, func(e staffeeapi.StaffAuditEntryResource) bool { return e.Target.Name.Value == "attributed-tag" })
	require.True(t, listed, "the StaffMember console lists the entry")
	assert.Equal(t, staffeeapi.StaffAuditActorKindService, shown.Actor.Kind)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, shown.Actor.Name.Value)

	res, err := h.env.SiteActor(t, fixtures.OwnerJohnEmail).SiteAuditList(t.Context(), siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	customerPage := res
	attributed := lo.Filter(customerPage.Items, func(e siteapi.SiteAuditEntryResource, _ int) bool { return e.Target.Name.Value == "attributed-tag" })
	for _, e := range attributed {
		assert.Equal(t, siteapi.SiteAuditActorKindStaff, e.Actor.Kind)
		assert.Equal(t, events.StaffLabel, e.Actor.Name.Value)
		assert.False(t, e.Actor.ID.IsSet() && e.Actor.ID.Value != "")
	}
}

func TestWithoutALicenseNothingIsServedAndTheModuleRefuses(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())

	assert.Equal(t, http.StatusNotFound, getIdentity(t, env.Internal, primary).Code, "the service endpoints are not routed without the license")
	_, err := env.ServiceCredentials.IssueServiceCredential(t.Context(), servicecredentials.IssueInput{Name: "s", Scopes: []servicecredentials.Scope{servicecredentials.ScopeUsageRead}})
	require.ErrorIs(t, err, workspaceoversight.ErrNotLicensed)
	_, err = env.ServiceCredentials.RotateServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID, nil)
	require.ErrorIs(t, err, workspaceoversight.ErrNotLicensed)
	require.ErrorIs(t, env.ServiceCredentials.RevokeServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID), workspaceoversight.ErrNotLicensed)
	_, err = env.ServiceCredentials.ServiceCredentials(t.Context(), "")
	require.ErrorIs(t, err, workspaceoversight.ErrNotLicensed)
	_, err = env.StaffEnterprise.UsageList(t.Context(), time.Now(), pagination.Params{})
	require.ErrorIs(t, err, workspaceoversight.ErrNotLicensed)
	_, err = env.StaffEnterprise.WorkspaceUsage(t.Context(), fixtures.AcmePublicID, time.Now())
	require.ErrorIs(t, err, workspaceoversight.ErrNotLicensed)
	_, err = env.ServiceCredentials.AuthenticateService(t.Context(), primary)
	require.ErrorIs(t, err, servicecredentials.ErrUnauthorized, "a valid value is refused without the license")
	assert.Equal(t, http.StatusNotFound, getIdentity(t, env.Server, primary).Code)
}
