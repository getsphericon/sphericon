package oauthserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ent/apitoken"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/oauthserver"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// consent plays the signed-in owner of workspace acme answering the consent screen.
func consent(t *testing.T, env *testhelper.TestEnv, in siteapi.SiteOAuthDecisionInput) *url.URL {
	t.Helper()
	return consentAs(t, env, fixtures.OwnerJohnEmail, in)
}

// consentAs plays the signed-in User email answering the consent screen.
func consentAs(t *testing.T, env *testhelper.TestEnv, email string, in siteapi.SiteOAuthDecisionInput) *url.URL {
	t.Helper()
	res, err := env.SiteActor(t, email).SiteOAuthDecide(t.Context(), &in)
	require.NoError(t, err)
	ok, isOK := res.(*siteapi.SiteOAuthDecisionResult)
	require.Truef(t, isOK, "got %T", res)
	u, err := url.Parse(ok.RedirectUrl)
	require.NoError(t, err)
	return u
}

func approval(scope string, allowSend bool) siteapi.SiteOAuthDecisionInput {
	return siteapi.SiteOAuthDecisionInput{
		ClientId:      fixtureClientID,
		RedirectUri:   fixtureRedirectURI,
		State:         siteapi.NewOptString("xyz"),
		CodeChallenge: oauth2.S256ChallengeFromVerifier(verifier),
		Scope:         siteapi.NewOptString(scope),
		WorkspaceId:   fixtures.AcmePublicID,
		Approve:       true,
		AllowSend:     siteapi.NewOptBool(allowSend),
	}
}

func exchange(t *testing.T, env *testhelper.TestEnv, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return serve(env, req)
}

func tokenForm(code string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {fixtureClientID},
		"redirect_uri":  {fixtureRedirectURI},
		"code_verifier": {verifier},
	}
}

func TestConsentScreenDescribesTheRequest(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{
		ClientId:    fixtureClientID,
		RedirectUri: fixtureRedirectURI,
		Scope:       siteapi.NewOptString("contacts:read emails:send sending_domains:write bogus"),
	})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteOAuthAuthorizationRequest)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Fixture Connector", got.ClientName)
	assert.Equal(t, []siteapi.GrantableScope{"contacts:read"}, got.Scopes)
	assert.Equal(t, []siteapi.SendScope{"emails:send", "mcp:send"}, got.SendScopes, "send-class scopes await an explicit opt-in, and bring the MCP send lock's key")
	assert.Equal(t, []siteapi.SetupScope{"sending_domains:write"}, got.SetupScopes, "setup scopes await an opt-in of their own and bring no mcp:send")
	assert.Equal(t, siteapi.SiteOAuthAuthorizationRequestRequiredPermissions{"sending_domains:write": siteapi.PermissionSendingDomainsManage}, got.RequiredPermissions,
		"the screen learns which requested scope needs a Permission to grant")
}

func TestConsentScreenRejectsMismatchedRedirect(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{
		ClientId:    fixtureClientID,
		RedirectUri: "https://evil.example/cb",
	})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDescribeBadRequest{}, res)
}

func TestConsentRequiresASession(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.SiteAnonymous(t).SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{ClientId: fixtureClientID, RedirectUri: fixtureRedirectURI})
	assert.Error(t, err)
}

func TestDenyingConsentSendsAccessDeniedBack(t *testing.T) {
	env := testhelper.Setup(t)
	in := approval("", false)
	in.Approve = false

	back := consent(t, env, in)

	assert.Equal(t, "access_denied", back.Query().Get("error"))
	assert.Equal(t, "xyz", back.Query().Get("state"))
	assert.Empty(t, back.Query().Get("code"))
}

func TestConsentToAWorkspaceTheUserDoesNotBelongToIsRefused(t *testing.T) {
	env := testhelper.Setup(t)
	in := approval("", false)
	in.WorkspaceId = "Someone1"

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDecide(t.Context(), &in)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecideNotFound{}, res)
}

func TestConsentAnAdminMayApprove(t *testing.T) {
	env := testhelper.Setup(t)
	admin := fixtures.AdminAliceEmail
	in := approval("contacts:write", false)

	res, err := env.SiteActor(t, admin).SiteOAuthDecide(t.Context(), &in)

	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteOAuthDecisionResult{}, res)
}

func TestTokenEndpointIssuesAConnectionOfTheApprovingUser(t *testing.T) {
	env := testhelper.Setup(t)
	back := consent(t, env, approval("contacts:read segments:read", false))
	require.Equal(t, "xyz", back.Query().Get("state"))

	rec := exchange(t, env, tokenForm(back.Query().Get("code")))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	var tok map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
	assert.Equal(t, "Bearer", tok["token_type"])
	assert.Equal(t, "contacts:read segments:read", tok["scope"])

	// It is a Connection: a scoped token of the workspace tied to the owner's Membership, and /mcp
	// accepts it.
	row, err := env.DB.ApiToken.Query().Where(apitoken.Name("Fixture Connector")).Only(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"contacts:read", "segments:read"}, row.Scopes)
	assert.Equal(t, int64(fixtures.AcmeID), row.WorkspaceID)
	assert.Equal(t, new(int64(fixtures.AcmeOwnerMembershipID)), row.MembershipID)
	assert.Nil(t, row.RevokedAt)

	session := env.MCPClient(t, tok["access_token"].(string))
	_, err = session.ListTools(t.Context(), nil)
	require.NoError(t, err)
}

func TestSendClassScopesNeedAnExplicitOptIn(t *testing.T) {
	env := testhelper.Setup(t)
	scope := "contacts:read emails:send broadcasts:send automations:publish"

	for allowSend, want := range map[bool][]string{
		false: {"contacts:read"},
		// contract-copy: the expected wire scopes of the issued token, an independent source of truth
		true: {"contacts:read", "emails:send", "broadcasts:send", "automations:publish", "mcp:send"},
	} {
		back := consent(t, env, approval(scope, allowSend))
		rec := exchange(t, env, tokenForm(back.Query().Get("code")))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var tok map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
		assert.Equal(t, strings.Join(want, " "), tok["scope"], "allowSend=%v", allowSend)
	}
}

// The setup scopes (Sending domains, Integrations) have a box of their own: only a client that
// asked for them and a user who ticked it get them, and neither the send box nor a send-class
// grant brings them or the other way round (ADR 0016).
func TestSetupScopesNeedTheirOwnExplicitOptIn(t *testing.T) {
	env := testhelper.Setup(t)
	asked := "contacts:read sending_domains:write integrations:write"

	for name, tc := range map[string]struct {
		scope      string
		allowSend  bool
		allowSetup bool
		want       string
	}{
		"no opt-in":                        {scope: asked, want: "contacts:read"},
		"the send box is not it":           {scope: asked, allowSend: true, want: "contacts:read"},
		"the setup box":                    {scope: asked, allowSetup: true, want: asked},
		"not asked, not granted":           {scope: "contacts:read", allowSetup: true, want: "contacts:read"},
		"setup beside send":                {scope: asked + " emails:send", allowSend: true, allowSetup: true, want: asked + " emails:send mcp:send"},
		"the setup box brings no mcp:send": {scope: asked + " emails:send", allowSetup: true, want: asked},
	} {
		t.Run(name, func(t *testing.T) {
			in := approval(tc.scope, tc.allowSend)
			in.AllowSetup = siteapi.NewOptBool(tc.allowSetup)
			back := consent(t, env, in)
			rec := exchange(t, env, tokenForm(back.Query().Get("code")))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			var tok map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))
			assert.ElementsMatch(t, strings.Fields(tc.want), strings.Fields(tok["scope"].(string)))
		})
	}
}

// A connector approved with the setup box can add a Sending domain through MCP; one approved
// without it is refused when it calls the tool (the tool is listed either way: only the
// send class is hidden without its key).
func TestApprovedSetupScopeLetsAConnectorAddASendingDomain(t *testing.T) {
	env := testhelper.Setup(t)

	for _, allowSetup := range []bool{false, true} {
		in := approval("sending_domains:read sending_domains:write", false)
		in.AllowSetup = siteapi.NewOptBool(allowSetup)
		back := consent(t, env, in)
		rec := exchange(t, env, tokenForm(back.Query().Get("code")))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var tok map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tok))

		session := env.MCPClient(t, tok["access_token"].(string))
		res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
			Name:      "sending_domains_create",
			Arguments: map[string]any{"domain": "oauth.example.com"},
		})
		require.NoError(t, err)
		assert.Equal(t, !allowSetup, res.IsError, "allowSetup=%v", allowSetup)
	}
}

func TestAuthorizationCodeIsSingleUse(t *testing.T) {
	env := testhelper.Setup(t)
	code := consent(t, env, approval("contacts:read", false)).Query().Get("code")

	first := exchange(t, env, tokenForm(code))
	second := exchange(t, env, tokenForm(code))

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusBadRequest, second.Code)
	assert.Contains(t, second.Body.String(), "invalid_grant")
}

func TestTokenEndpointChecksPKCEAndTheOriginalRequest(t *testing.T) {
	env := testhelper.Setup(t)

	for name, mutate := range map[string]func(url.Values){
		"wrong verifier":         func(f url.Values) { f.Set("code_verifier", strings.Repeat("a", 43)) },
		"missing verifier":       func(f url.Values) { f.Del("code_verifier") },
		"different redirect":     func(f url.Values) { f.Set("redirect_uri", "https://connector.example/other") },
		"different client":       func(f url.Values) { f.Set("client_id", "someone-else") },
		"unsupported grant":      func(f url.Values) { f.Set("grant_type", "password") },
		"unknown code":           func(f url.Values) { f.Set("code", "nope") },
		"client credentials too": func(f url.Values) { f.Set("grant_type", "client_credentials") },
	} {
		t.Run(name, func(t *testing.T) {
			code := consent(t, env, approval("contacts:read", false)).Query().Get("code")
			form := tokenForm(code)
			mutate(form)

			rec := exchange(t, env, form)

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.NotContains(t, rec.Body.String(), "access_token")
		})
	}
}

func TestResourceIndicatorMustBeTheMCPResource(t *testing.T) {
	env := testhelper.Setup(t)
	assertResourceIndicator(t, env, oauthserver.ResourceURL(appURL(t)), "https://elsewhere.example/mcp")
}

func TestSplitHostResourceIndicatorIsTheMCPHostResource(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.MCPURL = splitMCPURL }))
	// the issuer-host resource is no longer the protected resource
	assertResourceIndicator(t, env, oauthserver.ResourceURL(splitMCPURL), oauthserver.ResourceURL(appURL(t)))
}

func assertResourceIndicator(t *testing.T, env *testhelper.TestEnv, mcpResource, wrong string) {
	t.Helper()

	t.Run("authorize", func(t *testing.T) {
		rec := authorize(t, env, authorizeQuery(url.Values{"resource": {wrong}}))
		require.Equal(t, http.StatusFound, rec.Code)
		loc, err := url.Parse(rec.Header().Get("Location"))
		require.NoError(t, err)
		assert.Equal(t, "invalid_target", loc.Query().Get("error"))

		ok := authorize(t, env, authorizeQuery(url.Values{"resource": {mcpResource}}))
		loc, err = url.Parse(ok.Header().Get("Location"))
		require.NoError(t, err)
		assert.Empty(t, loc.Query().Get("error"))
	})

	t.Run("token", func(t *testing.T) {
		bad := tokenForm(consent(t, env, approval("contacts:read", false)).Query().Get("code"))
		bad.Set("resource", wrong)
		rec := exchange(t, env, bad)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Contains(t, rec.Body.String(), "invalid_target")
		assert.NotContains(t, rec.Body.String(), "access_token")

		good := tokenForm(consent(t, env, approval("contacts:read", false)).Query().Get("code"))
		good.Set("resource", mcpResource)
		assert.Equal(t, http.StatusOK, exchange(t, env, good).Code)

		omitted := tokenForm(consent(t, env, approval("contacts:read", false)).Query().Get("code"))
		assert.Equal(t, http.StatusOK, exchange(t, env, omitted).Code, "a missing resource stays accepted")
	})
}

// roundTripper serves requests straight from the in-memory app.
type roundTripper struct{ env *testhelper.TestEnv }

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	r.env.Server.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// A standard MCP client (the official go-sdk) discovers the server from the 401
// challenge, registers itself, runs the PKCE flow through the consent screen and
// ends up connected to /mcp with a Connection.
func TestStandardMCPClientConnectsThroughOAuth(t *testing.T) {
	env := testhelper.Setup(t)
	httpClient := &http.Client{Transport: roundTripper{env}}
	noFollow := &http.Client{
		Transport:     roundTripper{env},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	const callback = "https://agent.example/callback"

	handler, err := auth.NewAuthorizationCodeHandler(&auth.AuthorizationCodeHandlerConfig{
		DynamicClientRegistrationConfig: &auth.DynamicClientRegistrationConfig{
			Metadata: &oauthex.ClientRegistrationMetadata{
				ClientName:   "Test Agent",
				RedirectURIs: []string{callback},
			},
		},
		RedirectURL: callback,
		Client:      httpClient,
		// The user's browser: follow /oauth/authorize to the consent screen, then
		// approve as the signed-in owner and collect the code from the redirect.
		AuthorizationCodeFetcher: func(ctx context.Context, args *auth.AuthorizationArgs) (*auth.AuthorizationResult, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, args.URL, http.NoBody)
			if err != nil {
				return nil, err
			}
			resp, err := noFollow.Do(req)
			if err != nil {
				return nil, err
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusFound {
				t.Errorf("authorize answered %d", resp.StatusCode)
			}
			consentURL, err := url.Parse(resp.Header.Get("Location"))
			if err != nil {
				return nil, err
			}
			q := consentURL.Query()

			res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDecide(ctx, &siteapi.SiteOAuthDecisionInput{
				ClientId:      q.Get("client_id"),
				RedirectUri:   q.Get("redirect_uri"),
				State:         siteapi.NewOptString(q.Get("state")),
				CodeChallenge: q.Get("code_challenge"),
				Scope:         siteapi.NewOptString(q.Get("scope")),
				WorkspaceId:   fixtures.AcmePublicID,
				Approve:       true,
			})
			if err != nil {
				return nil, err
			}
			decision := res.(*siteapi.SiteOAuthDecisionResult)
			back, err := url.Parse(decision.RedirectUrl)
			if err != nil {
				return nil, err
			}
			return &auth.AuthorizationResult{Code: back.Query().Get("code"), State: back.Query().Get("state")}, nil
		},
	})
	require.NoError(t, err)

	transport := &mcp.StreamableClientTransport{
		Endpoint:             appURL(t) + "/mcp",
		HTTPClient:           httpClient,
		OAuthHandler:         handler,
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-agent", Version: "0"}, nil).Connect(t.Context(), transport, nil)
	require.NoError(t, err, "connect through OAuth")
	t.Cleanup(func() { cleanup.Close(session, "session") })

	tools, err := session.ListTools(t.Context(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, tools.Tools)

	row, err := env.DB.ApiToken.Query().Where(apitoken.Name("Test Agent")).Only(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, row.Scopes, "emails:send", "send-class scopes are opt-in")
	assert.NotContains(t, row.Scopes, "mcp:send", "so is the MCP send lock's key")
	assert.Contains(t, row.Scopes, "contacts:read")
}

// The default grant must cover the authoring tools the shipped prompts use, so a
// connector that asks for no particular scope can author (never send).
func TestDefaultGrantCoversTheAuthoringScopes(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteOAuthDescribe(t.Context(), siteapi.SiteOAuthDescribeParams{
		ClientId:    fixtureClientID,
		RedirectUri: fixtureRedirectURI,
	})
	require.NoError(t, err)
	got, ok := res.(*siteapi.SiteOAuthAuthorizationRequest)
	require.Truef(t, ok, "got %T", res)
	// contract-copy: the authoring scopes the shipped prompts use, a chosen subset of the contract's list
	assert.Subset(t, got.Scopes, []siteapi.GrantableScope{
		"contacts:read", "contacts:write", "events:read", "events:write",
		"segments:read", "segments:write", "broadcasts:read", "broadcasts:write",
		"automations:read", "automations:write", "enrollments:read", "templates:read", "templates:write",
		"webhooks:read", "webhooks:write", "webhooks:regenerate", "webhooks:test", "custom_fields:read", "sending_domains:read", "integrations:read",
	})
	assert.Empty(t, got.SendScopes)
	assert.NotContains(t, got.Scopes, siteapi.GrantableScope("tokens:write"), "a connector never mints credentials")
}
