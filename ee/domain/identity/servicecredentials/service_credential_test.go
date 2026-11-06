package servicecredentials_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ent"
	entcredential "github.com/getsphericon/sphericon/ent/servicecredential"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The module's tests run over the whole server, so a value it issues is seen to work, or not, at
// the service endpoints of the internal listener.

var (
	primary = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, fixtures.ServiceCredentialPrimarySecret)
	second  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialSecondPrefix, fixtures.ServiceCredentialSecondSecret)
)

// movableClock is the clock the server checks a credential's expiry and last use against.
type movableClock struct{ t time.Time }

func (c *movableClock) now() time.Time { return c.t }

type harness struct {
	env   *testhelper.TestEnv
	clock *movableClock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	c := &movableClock{t: time.Now()}
	return &harness{env: testhelper.Setup(t, testhelper.WithClock(c.now)), clock: c}
}

// identity reads the credential's own identity, failing unless it is answered.
func identity(t *testing.T, c *serviceapi.Client) *serviceapi.ServiceIdentityResource {
	t.Helper()
	res, err := c.ServiceIdentityGet(t.Context())
	require.NoError(t, err)
	return res
}

// getIdentity is a raw GET of the identity path on handler, with a bearer.
func getIdentity(t *testing.T, handler http.Handler, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/service/identity", nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestIssuingShowsTheValueOnceAndStoresOnlyAHash(t *testing.T) {
	h := newHarness(t)

	issued, err := h.env.ServiceCredentials.IssueServiceCredential(t.Context(), servicecredentials.IssueInput{
		Name: " service-c ", Scopes: []servicecredentials.Scope{servicecredentials.ScopeHoldsWrite, servicecredentials.ScopeUsageRead, servicecredentials.ScopeUsageRead},
	})
	require.NoError(t, err)

	assert.Equal(t, "service-c", issued.Name)
	assert.Equal(t, []servicecredentials.Scope{servicecredentials.ScopeHoldsWrite, servicecredentials.ScopeUsageRead}, issued.Scopes, "sorted, without duplicates")
	assert.True(t, strings.HasPrefix(issued.Value, "smsc_"))
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
		in   servicecredentials.IssueInput
		want error
	}{
		"no name":       {servicecredentials.IssueInput{Name: " ", Scopes: []servicecredentials.Scope{servicecredentials.ScopeUsageRead}}, servicecredentials.ErrInvalidCredential},
		"no scopes":     {servicecredentials.IssueInput{Name: "s"}, servicecredentials.ErrInvalidScope},
		"unknown scope": {servicecredentials.IssueInput{Name: "s", Scopes: []servicecredentials.Scope{"contacts:read"}}, servicecredentials.ErrInvalidScope},
		"past expiry":   {servicecredentials.IssueInput{Name: "s", Scopes: []servicecredentials.Scope{servicecredentials.ScopeUsageRead}, ExpiresAt: &past}, servicecredentials.ErrInvalidCredential},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.env.ServiceCredentials.IssueServiceCredential(t.Context(), tc.in)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestRotatingIssuesASecondCredentialAndLeavesTheFirstActive(t *testing.T) {
	h := newHarness(t)

	rotated, err := h.env.ServiceCredentials.RotateServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID, nil)
	require.NoError(t, err)

	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, rotated.Name)
	assert.Len(t, rotated.Scopes, 3, "the scopes carry over")
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, rotated.Value).Code)
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code, "no downtime: the old value still works until it is revoked")
}

func TestRotatingARevokedOrExpiredOrMissingCredentialIsRefused(t *testing.T) {
	h := newHarness(t)

	for _, id := range []int{fixtures.ServiceCredentialRevokedID, fixtures.ServiceCredentialExpiredID} {
		_, err := h.env.ServiceCredentials.RotateServiceCredential(t.Context(), int64(id), nil)
		require.ErrorIs(t, err, servicecredentials.ErrCredentialInactive)
	}
	_, err := h.env.ServiceCredentials.RotateServiceCredential(t.Context(), 999999, nil)
	require.ErrorIs(t, err, servicecredentials.ErrCredentialNotFound)
}

func TestARevokedCredentialIsRefusedAtOnceAndRevokingTwiceChangesNothing(t *testing.T) {
	h := newHarness(t)
	require.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code)

	require.NoError(t, h.env.ServiceCredentials.RevokeServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID))
	firstRevoked := h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialPrimaryID).RevokedAt
	h.clock.t = h.clock.t.Add(time.Hour)
	require.NoError(t, h.env.ServiceCredentials.RevokeServiceCredential(t.Context(), fixtures.ServiceCredentialPrimaryID))

	assert.Equal(t, http.StatusUnauthorized, getIdentity(t, h.env.Internal, primary).Code)
	assert.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, second).Code, "its sibling is untouched")
	assert.Equal(t, firstRevoked, h.env.DB.ServiceCredential.GetX(t.Context(), fixtures.ServiceCredentialPrimaryID).RevokedAt)
	require.ErrorIs(t, h.env.ServiceCredentials.RevokeServiceCredential(t.Context(), 999999), servicecredentials.ErrCredentialNotFound)
}

func TestTheListShowsEachCredentialsStateAndWhenItWasLastUsed(t *testing.T) {
	h := newHarness(t)
	before, err := h.env.ServiceCredentials.ServiceCredentials(t.Context(), fixtures.ServiceCredentialPrimaryName)
	require.NoError(t, err)
	require.NotEmpty(t, before)
	for _, c := range before {
		assert.Nil(t, c.LastUsedAt, "never used")
	}

	require.Equal(t, http.StatusOK, getIdentity(t, h.env.Internal, primary).Code)

	listed, err := h.env.ServiceCredentials.ServiceCredentials(t.Context(), fixtures.ServiceCredentialPrimaryName)
	require.NoError(t, err)
	for _, c := range listed {
		assert.Equal(t, fixtures.ServiceCredentialPrimaryName, c.Name)
	}
	used := lo.Filter(listed, func(c *ent.ServiceCredential, _ int) bool { return c.LastUsedAt != nil })
	require.Len(t, used, 1, "only the credential that was used")
	assert.EqualValues(t, fixtures.ServiceCredentialPrimaryID, used[0].ID)
	assert.WithinDuration(t, h.clock.now(), *used[0].LastUsedAt, time.Millisecond, "at the clock's time")

	all, err := h.env.ServiceCredentials.ServiceCredentials(t.Context(), "")
	require.NoError(t, err)
	assert.Greater(t, len(all), len(listed), "no name lists every service's")
}

func TestTheStoredCredentialsHoldNoClearValue(t *testing.T) {
	h := newHarness(t)

	n := h.env.DB.ServiceCredential.Query().Where(entcredential.SecretHashContains(fixtures.ServiceCredentialPrimarySecret)).CountX(t.Context())

	assert.Zero(t, n)
}
