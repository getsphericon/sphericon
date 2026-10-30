package server_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The refusals the server's error handler renders (review rule 15) answer the same status and
// body on /site and /api: the handlers return the module's error and the problem is written once.

// answer is the status and raw body of the last response a client received.
type answer struct {
	status int
	body   string
}

// answerCapture keeps the wire answer, which the typed client decodes away.
type answerCapture struct {
	inner ht.Client
	got   *answer
}

func (c answerCapture) Do(r *http.Request) (*http.Response, error) {
	res, err := c.inner.Do(r)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	*c.got = answer{status: res.StatusCode, body: string(raw)}
	res.Body = io.NopCloser(strings.NewReader(string(raw)))
	return res, nil
}

// capture wraps a transport so that got holds each answer.
func capture(got *answer) func(ht.Client) ht.Client {
	return func(inner ht.Client) ht.Client { return answerCapture{inner: inner, got: got} }
}

func TestARefusedInputAnswers422AlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeTokensWrite), string(externalapi.ApiTokenScopeContactsRead)), capture(&ext))

	_, err := siteClient.SiteTokensCreate(t.Context(), &siteapi.SiteCreateTokenInput{Name: "   ", Scopes: []siteapi.ApiTokenScope{siteapi.ApiTokenScopeContactsRead}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = extClient.AuthTokensCreate(t.Context(), &externalapi.CreateApiTokenInput{Name: "   ", Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead}})
	require.NoError(t, err)

	want := `{"status":422,"title":"Unprocessable Entity","detail":"name must not be empty","errors":{"name":["name must not be empty"]}}`
	assert.Equal(t, http.StatusUnprocessableEntity, site.status)
	assert.JSONEq(t, want, site.body)
	assert.Equal(t, http.StatusUnprocessableEntity, ext.status)
	assert.JSONEq(t, want, ext.body)
}

func TestARemovalOfARowInUseAnswers409AlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeContactsWrite)), capture(&ext))

	_, err := siteClient.SiteTagsDelete(t.Context(), siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.TagVipID})
	require.NoError(t, err)
	_, err = extClient.TagsDelete(t.Context(), externalapi.TagsDeleteParams{ID: fixtures.TagVipID})
	require.NoError(t, err)

	assert.Equal(t, http.StatusConflict, site.status)
	assert.Equal(t, http.StatusConflict, ext.status)
	assert.JSONEq(t, site.body, ext.body)
	assert.Contains(t, site.body, `"usedBy":[{"kind":`)
	assert.Contains(t, site.body, `"name":"`+fixtures.RuleVipOnlyName+`"`)
}

// /api has no Workspace in its paths (a token is bound to one), so only /site can address a
// Workspace the caller cannot reach; an unknown one and a foreign one answer alike.
func TestAWorkspaceTheCallerCannotReachAnswers404AlikeOnEveryOperation(t *testing.T) {
	env := testhelper.Setup(t)
	var unknown, foreign answer
	unknownClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&unknown))
	foreignClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&foreign))

	_, err := unknownClient.SiteTagsDelete(t.Context(), siteapi.SiteTagsDeleteParams{WorkspaceId: "Unknown0", ID: fixtures.TagVipID})
	require.NoError(t, err)
	_, err = foreignClient.SiteTokensList(t.Context(), siteapi.SiteTokensListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)

	want := `{"status":404,"title":"Not Found","detail":"workspace not found"}`
	assert.Equal(t, http.StatusNotFound, unknown.status)
	assert.JSONEq(t, want, unknown.body)
	assert.Equal(t, http.StatusNotFound, foreign.status)
	assert.JSONEq(t, want, foreign.body)
}

// A Membership whose Role lacks the Permission an operation needs answers 403 with code
// permission_required (ADR 0061), apart from the other 403s. Permissions are a /site matter: an
// API token's counterpart is its scope.
func TestARoleWithoutThePermissionAnswers403WithItsCode(t *testing.T) {
	env := testhelper.Setup(t)
	var got answer
	member := env.SiteActorVia(t, fixtures.MemberMaryEmail, capture(&got))

	_, err := member.SiteTokensCreate(t.Context(), &siteapi.SiteCreateTokenInput{Name: "script", Scopes: []siteapi.ApiTokenScope{siteapi.ApiTokenScopeContactsRead}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	want := `{"code":"permission_required","status":403,"title":"Forbidden","detail":"your role in this workspace does not allow managing API tokens"}`
	assert.Equal(t, http.StatusForbidden, got.status)
	assert.JSONEq(t, want, got.body)
}

// kindSurfaces is one Acme owner on /site and an Acme token on /api, each client keeping the last
// answer it received.
type kindSurfaces struct {
	site       *siteapi.Client
	ext        *externalapi.Client
	siteAnswer *answer
	extAnswer  *answer
}

func newKindSurfaces(t *testing.T) kindSurfaces {
	t.Helper()
	env := testhelper.Setup(t)
	k := kindSurfaces{siteAnswer: &answer{}, extAnswer: &answer{}}
	k.site = env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(k.siteAnswer))
	k.ext = env.ExternalVia(t, env.ScopedBearer(t,
		string(externalapi.ApiTokenScopeTemplatesWrite), string(externalapi.ApiTokenScopeTemplatesRead),
		string(externalapi.ApiTokenScopeBroadcastsWrite), string(externalapi.ApiTokenScopeAutomationsWrite),
		string(externalapi.ApiTokenScopeWebhooksWrite), string(externalapi.ApiTokenScopeIntegrationsWrite),
	), capture(k.extAnswer))
	return k
}

// A module's sentinel refusal carries its kind, and the error handler answers the kind: not found
// 404, refused 422 (under its field when it names one, with the detail it was made With), conflict
// 409. /site and /api answer the same status and body.
func TestAModuleRefusalAnswersByItsKindAlikeOnBothSurfaces(t *testing.T) {
	const missing = 99999
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	untagged := []siteapi.SiteRuleAction{{Type: siteapi.SiteRuleActionTypeApplyTag}}
	untaggedExt := []externalapi.RuleAction{{Type: externalapi.RuleActionTypeApplyTag}}

	cases := []struct {
		name   string
		site   func(ctx context.Context, c *siteapi.Client) error
		ext    func(ctx context.Context, c *externalapi.Client) error
		status int
		want   string
	}{
		{
			name: "not found",
			site: func(ctx context.Context, c *siteapi.Client) error {
				_, err := c.SiteTemplatesGet(ctx, siteapi.SiteTemplatesGetParams{WorkspaceId: ws, ID: missing})
				return err
			},
			ext: func(ctx context.Context, c *externalapi.Client) error {
				_, err := c.TemplatesGet(ctx, externalapi.TemplatesGetParams{ID: missing})
				return err
			},
			status: http.StatusNotFound,
			want:   `{"status":404,"title":"Not Found","detail":"template not found"}`,
		},
		{
			name: "refused",
			site: func(ctx context.Context, c *siteapi.Client) error {
				_, err := c.SiteBroadcastsUpdate(ctx, &siteapi.SiteUpdateBroadcastInput{Name: siteapi.NewOptString("x")},
					siteapi.SiteBroadcastsUpdateParams{WorkspaceId: ws, ID: fixtures.BroadcastSentID})
				return err
			},
			ext: func(ctx context.Context, c *externalapi.Client) error {
				_, err := c.BroadcastsUpdate(ctx, &externalapi.UpdateBroadcastInput{Name: externalapi.NewOptString("x")},
					externalapi.BroadcastsUpdateParams{ID: fixtures.BroadcastSentID})
				return err
			},
			status: http.StatusUnprocessableEntity,
			want:   `{"status":422,"title":"Unprocessable Entity","detail":"only draft broadcasts can be changed"}`,
		},
		{
			name: "refused with a detail",
			site: func(ctx context.Context, c *siteapi.Client) error {
				_, err := c.SiteRulesCreate(ctx, &siteapi.SiteCreateRuleInput{Name: "x", TriggerEvents: []string{"a"}, Actions: untagged},
					siteapi.SiteRulesCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(ctx context.Context, c *externalapi.Client) error {
				_, err := c.RulesCreate(ctx, &externalapi.CreateRuleInput{Name: "x", TriggerEvents: []string{"a"}, Actions: untaggedExt})
				return err
			},
			status: http.StatusUnprocessableEntity,
			want:   `{"status":422,"title":"Unprocessable Entity","detail":"invalid rule: action 1: a tag action needs a tag"}`,
		},
		{
			name: "refused on a field",
			site: func(ctx context.Context, c *siteapi.Client) error {
				_, err := c.SiteWebhooksCreate(ctx, &siteapi.SiteCreateWebhookEndpointInput{URL: "not-a-url"},
					siteapi.SiteWebhooksCreateParams{WorkspaceId: ws})
				return err
			},
			ext: func(ctx context.Context, c *externalapi.Client) error {
				_, err := c.WebhooksCreate(ctx, &externalapi.CreateWebhookInput{URL: "not-a-url"})
				return err
			},
			status: http.StatusUnprocessableEntity,
			want: `{"status":422,"title":"Unprocessable Entity","detail":"url must be an absolute http or https URL",` +
				`"errors":{"url":["url must be an absolute http or https URL"]}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := newKindSurfaces(t)

			require.NoError(t, tc.site(t.Context(), k.site))
			require.NoError(t, tc.ext(t.Context(), k.ext))

			assert.Equal(t, tc.status, k.siteAnswer.status)
			assert.JSONEq(t, tc.want, k.siteAnswer.body)
			assert.Equal(t, tc.status, k.extAnswer.status)
			assert.JSONEq(t, tc.want, k.extAnswer.body)
		})
	}
}

func TestAConflictRefusalAnswers409AlikeOnBothSurfaces(t *testing.T) {
	k := newKindSurfaces(t)
	inbox := siteapi.SiteIntegrationConfigInput{OneOf: siteapi.NewSiteInboxConfigInputSiteIntegrationConfigInputSum(
		siteapi.SiteInboxConfigInput{Kind: siteapi.SiteInboxConfigInputKindInbox})}
	params := siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID}
	kept, err := k.site.SiteIntegrationsCreate(t.Context(), &siteapi.SiteCreateIntegrationInput{Name: "Kept inbox", Config: inbox}, params)
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, kept)

	_, err = k.site.SiteIntegrationsCreate(t.Context(), &siteapi.SiteCreateIntegrationInput{Name: "Second inbox", Config: inbox}, params)
	require.NoError(t, err)
	_, err = k.ext.IntegrationsCreate(t.Context(), &externalapi.CreateIntegrationInput{Name: "Second inbox", Config: externalapi.IntegrationConfigInput{
		OneOf: externalapi.NewInboxConfigInputIntegrationConfigInputSum(externalapi.InboxConfigInput{Kind: externalapi.InboxConfigInputKindInbox}),
	}})
	require.NoError(t, err)

	want := `{"status":409,"title":"Conflict","detail":"a workspace has at most one Test inbox: use the one it has, or archive it first"}`
	assert.Equal(t, http.StatusConflict, k.siteAnswer.status)
	assert.JSONEq(t, want, k.siteAnswer.body)
	assert.Equal(t, http.StatusConflict, k.extAnswer.status)
	assert.JSONEq(t, want, k.extAnswer.body)
}

// Content that cannot render is the author's to fix: 422 with the reason, alike on both surfaces.
func TestContentThatCannotRenderAnswers422AlikeOnBothSurfaces(t *testing.T) {
	k := newKindSurfaces(t)
	const broken = "{% if %}"

	_, err := k.site.SiteTemplatesCreate(t.Context(), &siteapi.SiteCreateEmailTemplateInput{Name: "Broken", Body: siteapi.NewOptString(broken)},
		siteapi.SiteTemplatesCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = k.ext.TemplatesCreate(t.Context(), &externalapi.CreateTemplateInput{Name: "Broken", Body: externalapi.NewOptString(broken)})
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnprocessableEntity, k.siteAnswer.status)
	assert.Equal(t, http.StatusUnprocessableEntity, k.extAnswer.status)
	assert.JSONEq(t, k.siteAnswer.body, k.extAnswer.body)
	assert.Contains(t, k.siteAnswer.body, `"detail":"message content cannot be rendered: `)
}

// A blank Workspace name is refused by accounts on its field, as a create and as a rename (/site
// only: /api has no Workspace operations).
func TestABlankWorkspaceNameIsRefusedOnItsField(t *testing.T) {
	k := newKindSurfaces(t)

	_, err := k.site.SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: "   "})
	require.NoError(t, err)
	created := *k.siteAnswer
	_, err = k.site.SiteWorkspacesUpdate(t.Context(), &siteapi.SiteUpdateWorkspaceInput{Name: "   "},
		siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	renamed := *k.siteAnswer

	want := `{"status":422,"title":"Unprocessable Entity","detail":"name must not be empty","errors":{"name":["name must not be empty"]}}`
	assert.Equal(t, http.StatusUnprocessableEntity, created.status)
	assert.JSONEq(t, want, created.body)
	assert.Equal(t, http.StatusUnprocessableEntity, renamed.status)
	assert.JSONEq(t, want, renamed.body)
}

// A conflict that names a request field answers 409 with the message under that field, alike on
// both surfaces: an alias key another Contact already has.
func TestAConflictOnAFieldAnswers409UnderTheFieldAlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeContactsWrite)), capture(&ext))

	_, err := siteClient.SiteContactsCreate(t.Context(), &siteapi.SiteCreateContactInput{Email: siteapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)},
		siteapi.SiteContactsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = extClient.ContactsCreate(t.Context(), &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress(fixtures.ContactAliceEmail)})
	require.NoError(t, err)

	want := `{"status":409,"title":"Conflict","detail":"email already exists","errors":{"email":["email already exists"]}}`
	assert.Equal(t, http.StatusConflict, site.status)
	assert.JSONEq(t, want, site.body)
	assert.Equal(t, http.StatusConflict, ext.status)
	assert.JSONEq(t, want, ext.body)
}

// A Test send of a Workspace whose sending is held is refused with the hold's reason (422), alike
// on both surfaces: the outbound module refuses it and the error handler renders it.
func TestAHeldTestSendAnswers422AlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	env.SuspendWorkspace(t.Context(), fixtures.AcmeID, time.Now())
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t,
		string(externalapi.ApiTokenScopeBroadcastsRead), string(externalapi.ApiTokenScopeBroadcastsWrite)), capture(&ext))

	_, err := siteClient.SiteBroadcastsTestSend(t.Context(), &siteapi.SiteTestSendBroadcastInput{Email: fixtures.OwnerJohnEmail},
		siteapi.SiteBroadcastsTestSendParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)
	_, err = extClient.BroadcastsTestSend(t.Context(), &externalapi.TestSendBroadcastInput{Email: fixtures.OwnerJohnEmail},
		externalapi.BroadcastsTestSendParams{ID: fixtures.BroadcastDraftID})
	require.NoError(t, err)

	want := `{"status":422,"title":"Unprocessable Entity","detail":"the message was not sent: sending is suspended for this workspace"}`
	assert.Equal(t, http.StatusUnprocessableEntity, site.status)
	assert.JSONEq(t, want, site.body)
	assert.Equal(t, http.StatusUnprocessableEntity, ext.status)
	assert.JSONEq(t, want, ext.body)
}

// Restoring an archived Sending domain whose domain was added again is refused (409), alike on
// both surfaces.
func TestRestoringASendingDomainAddedAgainAnswers409AlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeSendingDomainsWrite)), capture(&ext))
	again, err := siteClient.SiteSendingDomainsCreate(t.Context(), &siteapi.SiteCreateSendingDomainInput{Domain: fixtures.SendingDomainArchivedDomain},
		siteapi.SiteSendingDomainsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteSendingDomainResource{}, again)

	_, err = siteClient.SiteSendingDomainsUnarchive(t.Context(), siteapi.SiteSendingDomainsUnarchiveParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.SendingDomainArchivedID})
	require.NoError(t, err)
	_, err = extClient.SendingDomainsUnarchive(t.Context(), externalapi.SendingDomainsUnarchiveParams{ID: fixtures.SendingDomainArchivedID})
	require.NoError(t, err)

	want := `{"status":409,"title":"Conflict","detail":"this domain was added again; archive that one first"}`
	assert.Equal(t, http.StatusConflict, site.status)
	assert.JSONEq(t, want, site.body)
	assert.Equal(t, http.StatusConflict, ext.status)
	assert.JSONEq(t, want, ext.body)
}
