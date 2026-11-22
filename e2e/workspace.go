//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/usecase/workspaceview"
)

// tokenScopes are everything the scenario steps need; a test's token carries them all.
var tokenScopes = scopeStrings(
	externalapi.ApiTokenScopeIntegrationsRead, externalapi.ApiTokenScopeIntegrationsWrite,
	externalapi.ApiTokenScopeSendingDomainsRead, externalapi.ApiTokenScopeSendingDomainsWrite,
	externalapi.ApiTokenScopeContactsRead, externalapi.ApiTokenScopeContactsWrite, externalapi.ApiTokenScopeContactsExport,
	externalapi.ApiTokenScopeBroadcastsRead, externalapi.ApiTokenScopeBroadcastsWrite, externalapi.ApiTokenScopeBroadcastsSend,
	externalapi.ApiTokenScopeAutomationsRead, externalapi.ApiTokenScopeAutomationsWrite, externalapi.ApiTokenScopeAutomationsPublish,
	externalapi.ApiTokenScopeEventsWrite, externalapi.ApiTokenScopeWebhooksWrite,
)

// scopeStrings names scopes by the contract's generated constants; the token store takes strings.
func scopeStrings(scopes ...externalapi.ApiTokenScope) []string {
	return lo.Map(scopes, func(s externalapi.ApiTokenScope, _ int) string { return string(s) })
}

// Workspace is one test's own tenant: a fresh User, Workspace and API token made with
// the product's own domain functions, a unique Sending domain and a client for the
// external API. Everything is Workspace-scoped, so tests are
// independent and may run in parallel.
type Workspace struct {
	t   testing.TB
	env *Env
	api *externalapi.Client
	// collect is the tracking ingestion API, carrying the Workspace's collect key.
	collect *collectapi.Client
	// site is the SPA's API, signed in as the owner.
	site     *siteapi.Client
	publicID string

	// Domain is this Workspace's unique Sending domain (the dev DKIM lookup resolves
	// by name alone, so no two tests may share one).
	Domain string
	// FromEmail is the address mail is sent from, on Domain.
	FromEmail string
	// FromName is the sender display name configured on the Integration.
	FromName string

	// OwnerEmail is the address of the User who created the Workspace.
	OwnerEmail string

	// Inbox is the only way a scenario observes mail.
	Inbox *Inbox

	scoped *ent.Scoped
}

// uniq is a short random token for unique domains and recipients.
func uniq() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type bearer string

func (b bearer) BearerAuth(context.Context, externalapi.OperationName) (externalapi.BearerAuth, error) {
	return externalapi.BearerAuth{Token: string(b)}, nil
}

// collectKey is the x-collect-key of a Workspace, the credential of the tracking snippet.
type collectKey string

func (k collectKey) ApiKeyAuth(context.Context, collectapi.OperationName) (collectapi.ApiKeyAuth, error) {
	return collectapi.ApiKeyAuth{APIKey: string(k)}, nil
}

// NewWorkspace is the arrange step: a User, a Workspace and its first API token, made
// with internal/domain/identity/users, internal/usecase/workspaceview and internal/domain/identity/apitokens (the bootstrap token is not used, it
// only addresses the oldest Workspace). Its Inbox deletes the test's own mail on cleanup.
func (e *Env) NewWorkspace(tb testing.TB) *Workspace {
	tb.Helper()
	ctx := tb.Context()
	id := uniq()

	user, err := e.users.CreateUser(ctx, users.CreateUserInput{Name: "E2E " + id, Email: "owner-" + id + "@e2e.test", PasswordHash: "!"})
	require.NoError(tb, err)
	created, err := e.creation.Create(ctx, workspaceview.CreateInput{UserID: user.ID, Name: "e2e " + id})
	require.NoError(tb, err)
	ws := created.Summary.Workspace
	member, err := e.memberships.Scope(ctx, user.ID, ws.PublicID)
	require.NoError(tb, err)
	scoped := member.Scope
	minted, err := e.tokens.Mint(ctx, scoped, apitokens.Input{Name: "e2e", Scopes: tokenScopes})
	require.NoError(tb, err)

	api, err := externalapi.NewClient(e.BaseURL+"/api", bearer(minted.Value))
	require.NoError(tb, err)

	collect, err := collectapi.NewClient(e.BaseURL+"/collect", collectKey(ws.CollectKey))
	require.NoError(tb, err)

	cookie, err := e.sessions.Issue(user, auth.Session{Method: auth.MethodPassword})
	require.NoError(tb, err)
	site, err := siteapi.NewClient(e.BaseURL+"/site", cookieSource(cookie.Value))
	require.NoError(tb, err)

	w := &Workspace{
		t: tb, env: e, api: api, collect: collect, site: site, publicID: ws.PublicID, OwnerEmail: user.Email,
		Domain:    id + ".e2e.test",
		FromEmail: "news@" + id + ".e2e.test",
		FromName:  "E2E News",
		scoped:    scoped,
	}
	w.Inbox = newInbox(tb, e.mailpit)
	return w
}

// Ready is the common setup: an Integration pointing at Mailpit, a verified Sending domain and
// the postal address marketing email is armed with (ADR 0156), so a Broadcast can leave. Scenarios
// that need less call the steps themselves.
func (w *Workspace) Ready() *Workspace {
	w.t.Helper()
	w.CreateMailpitIntegration()
	w.AddVerifiedSendingDomain()
	w.SetPostalAddress(PostalAddress)
	return w
}

// PostalAddress is the address Ready gives a Workspace, which every marketing email's footer
// carries.
const PostalAddress = "1 E2E Street\nTestville"

// SetPostalAddress saves the Workspace's postal address as its owner does in the settings: only a
// person sets it, so the step goes through the SPA's API.
func (w *Workspace) SetPostalAddress(address string) {
	w.t.Helper()
	ctx := w.t.Context()
	params := siteapi.SiteWorkspacesGetParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)}
	read, err := w.site.SiteWorkspacesGet(ctx, params)
	require.NoError(w.t, err, "read the workspace")
	_, err = w.site.SiteWorkspacesUpdate(ctx, &siteapi.SiteUpdateWorkspaceInput{
		Revision: read.Revision, Name: siteapi.Name(read.Name), PostalAddress: siteapi.NewOptString(address),
	}, siteapi.SiteWorkspacesUpdateParams{WorkspaceId: siteapi.WorkspaceId(w.publicID)})
	require.NoError(w.t, err, "set the postal address")
}

// NewRecipient returns an address unique to this call, so mail can be matched to the test.
func (w *Workspace) NewRecipient() string {
	return "to-" + uniq() + "@recipients.e2e.test"
}

// ok unwraps an API result of the expected type or fails the test with whatever
// the API answered instead (an RFC 7807 problem, typically).
func ok[T any](tb testing.TB, step string, res any, err error) *T {
	tb.Helper()
	require.NoError(tb, err, step)
	v, isT := res.(*T)
	require.True(tb, isT, "%s: unexpected response %T: %+v", step, res, res)
	return v
}

// String names the Workspace in failures.
func (w *Workspace) String() string { return "e2e workspace " + w.Domain }

// AddOwner makes a second owner of the Workspace and returns the Membership ids of all
// its owners (the founder first).
func (w *Workspace) AddOwner() []int64 {
	w.t.Helper()
	ctx := w.t.Context()
	id := uniq()
	user, err := w.env.users.CreateUser(ctx, users.CreateUserInput{Name: "E2E co-owner " + id, Email: "co-owner-" + id + "@e2e.test", PasswordHash: "!"})
	require.NoError(w.t, err)
	// An owner change takes its Workspace feed position inside a transaction.
	err = w.env.bus.WithinScopedTx(ctx, w.scoped, func(ts *ent.Scoped, _ events.Publisher) error {
		_, cerr := ts.Membership().Create().SetUserID(user.ID).SetRole(membership.RoleOwner).SetSource(membership.SourceCreator).Save(ctx)
		return cerr
	})
	require.NoError(w.t, err)
	return w.OwnerMembershipIDs()
}

// OwnerMembershipIDs lists the Membership ids of the Workspace's owners, ascending.
func (w *Workspace) OwnerMembershipIDs() []int64 {
	w.t.Helper()
	ids, err := w.scoped.Membership().Query().
		Where(memberships.OwnerRole()).
		Order(ent.Asc(membership.FieldID)).
		IDs(w.t.Context())
	require.NoError(w.t, err)
	return ids
}

// systemOwner changes Memberships as an owner would, with the system as the actor of its Audit
// entries.
var systemOwner = memberships.Caller{Actor: events.Actor{Kind: events.ActorSystem}, Role: membership.RoleOwner}

// DemoteConcurrently demotes each Membership to member, all at once, as an owner would,
// and returns the error of each attempt.
func (w *Workspace) DemoteConcurrently(ids ...int64) []error {
	w.t.Helper()
	return w.concurrently(len(ids), func(i int) error {
		_, err := w.env.memberships.ChangeRole(w.t.Context(), w.scoped, systemOwner,
			memberships.ChangeRoleInput{ID: ids[i], Role: membership.RoleMember})
		return err
	})
}

// RemoveConcurrently removes each Membership, all at once, as an owner would, and returns
// the error of each attempt.
func (w *Workspace) RemoveConcurrently(ids ...int64) []error {
	w.t.Helper()
	return w.concurrently(len(ids), func(i int) error {
		return w.env.memberships.Remove(w.t.Context(), w.scoped, systemOwner, ids[i])
	})
}

// DemoteAndRemoveConcurrently demotes the first Membership and removes the second at the
// same moment, and returns the error of each attempt.
func (w *Workspace) DemoteAndRemoveConcurrently(demote, remove int64) []error {
	w.t.Helper()
	return w.concurrently(2, func(i int) error {
		if i == 0 {
			_, err := w.env.memberships.ChangeRole(w.t.Context(), w.scoped, systemOwner,
				memberships.ChangeRoleInput{ID: demote, Role: membership.RoleMember})
			return err
		}
		return w.env.memberships.Remove(w.t.Context(), w.scoped, systemOwner, remove)
	})
}

// concurrently releases n attempts at the same moment and returns each one's error.
func (w *Workspace) concurrently(n int, attempt func(i int) error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			<-start
			errs[i] = attempt(i)
		})
	}
	close(start)
	wg.Wait()
	return errs
}
