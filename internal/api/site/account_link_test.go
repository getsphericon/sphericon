package site_test

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const accountLinkAddress = "https://accounts.example.test/open"

var accountLinkNow = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// receiver is the other side of the link: it knows only the address it was given
// and the key set the app publishes, never a shared secret.
type receiver struct{ keys jose.JSONWebKeySet }

type linkClaims struct {
	Subject     string `json:"sub"`
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
}

// verify checks the token the way a receiver would: signature against the
// published key set, then audience and expiry at the given time.
func (r receiver) verify(token, audience string, at time.Time) (linkClaims, error) {
	parsed, err := josejwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.EdDSA})
	if err != nil {
		return linkClaims{}, err
	}
	keys := r.keys.Key(parsed.Headers[0].KeyID)
	if len(keys) == 0 {
		return linkClaims{}, jose.ErrUnsupportedKeyType
	}
	var std josejwt.Claims
	var custom linkClaims
	if err := parsed.Claims(keys[0], &std, &custom); err != nil {
		return linkClaims{}, err
	}
	if err := std.ValidateWithLeeway(josejwt.Expected{AnyAudience: josejwt.Audience{audience}, Time: at}, 0); err != nil {
		return linkClaims{}, err
	}
	return custom, nil
}

func accountLinkEnv(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	seed := make([]byte, 32)
	_, err := rand.Read(seed)
	require.NoError(t, err)
	return testhelper.Setup(t,
		testhelper.WithClock(func() time.Time { return accountLinkNow }),
		testhelper.WithConfig(func(c *config.Config) {
			c.AccountLinkURL = accountLinkAddress
			c.AccountLinkSigningKey = base64.StdEncoding.EncodeToString(seed)
		}))
}

func publishedKeys(t *testing.T, env *testhelper.TestEnv) receiver {
	t.Helper()
	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/jwks.json", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var set jose.JSONWebKeySet
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &set))
	require.Len(t, set.Keys, 1)
	assert.True(t, set.Keys[0].IsPublic(), "the key set must carry no private key")
	return receiver{keys: set}
}

func mintAccountLink(t *testing.T, env *testhelper.TestEnv, email string) siteapi.SiteWorkspacesAccountLinkRes {
	t.Helper()
	res, err := env.SiteActor(t, email).SiteWorkspacesAccountLink(t.Context(), siteapi.SiteWorkspacesAccountLinkParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return res
}

func TestOwnerOpensAccountLinkWithVerifiableToken(t *testing.T) {
	env := accountLinkEnv(t)

	link, ok := mintAccountLink(t, env, fixtures.OwnerJohnEmail).(*siteapi.SiteAccountLinkResource)
	require.True(t, ok)
	assert.Equal(t, accountLinkAddress, link.URL)
	assert.True(t, time.Time(link.ExpiresAt).Equal(accountLinkNow.Add(2*time.Minute)))

	claims, err := publishedKeys(t, env).verify(link.Token, accountLinkAddress, accountLinkNow.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, strconv.Itoa(fixtures.OwnerJohnID), claims.Subject)
	assert.Equal(t, strconv.Itoa(fixtures.AcmeID), claims.WorkspaceID)
	assert.Equal(t, "owner", claims.Role)
}

func TestAccountLinkTokenExpiresAndIsBoundToAudience(t *testing.T) {
	env := accountLinkEnv(t)
	link := mintAccountLink(t, env, fixtures.OwnerJohnEmail).(*siteapi.SiteAccountLinkResource)
	r := publishedKeys(t, env)

	_, err := r.verify(link.Token, accountLinkAddress, accountLinkNow.Add(3*time.Minute))
	require.ErrorIs(t, err, josejwt.ErrExpired)

	_, err = r.verify(link.Token, "https://other.example.test", accountLinkNow)
	require.ErrorIs(t, err, josejwt.ErrInvalidAudience)
}

func TestAccountLinkIsForTheOwnerOnly(t *testing.T) {
	env := accountLinkEnv(t)

	assert.IsType(t, &siteapi.SiteWorkspacesAccountLinkForbidden{}, mintAccountLink(t, env, fixtures.MemberMaryEmail))
	assert.IsType(t, &siteapi.SiteWorkspacesAccountLinkNotFound{}, mintAccountLink(t, env, fixtures.OutsiderOscarEmail))

	assert.True(t, workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID).AccountLinkAvailable)
	assert.False(t, workspaceOf(t, env.SiteActor(t, fixtures.MemberMaryEmail), fixtures.AcmePublicID).AccountLinkAvailable)
}

func TestAccountLinkIsAbsentWhenNotConfigured(t *testing.T) {
	env := testhelper.Setup(t)

	assert.IsType(t, &siteapi.SiteWorkspacesAccountLinkNotFound{}, mintAccountLink(t, env, fixtures.OwnerJohnEmail))
	assert.False(t, workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID).AccountLinkAvailable)

	rec := httptest.NewRecorder()
	env.Server.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/.well-known/jwks.json", nil))
	assert.NotEqual(t, http.StatusOK, rec.Code, "no key set is published without the configuration")
}
