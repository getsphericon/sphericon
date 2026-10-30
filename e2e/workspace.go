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
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/events"
)

// tokenScopes are everything the scenario steps need; a test's token carries them all.
var tokenScopes = scopeStrings(
	externalapi.ApiTokenScopeIntegrationsRead, externalapi.ApiTokenScopeIntegrationsWrite,
	externalapi.ApiTokenScopeSendingDomainsRead, externalapi.ApiTokenScopeSendingDomainsWrite,
	externalapi.ApiTokenScopeContactsRead, externalapi.ApiTokenScopeContactsWrite, externalapi.ApiTokenScopeContactsExport,
	externalapi.ApiTokenScopeBroadcastsRead, externalapi.ApiTokenScopeBroadcastsWrite, externalapi.ApiTokenScopeBroadcastsSend,
	externalapi.ApiTokenScopeAutomationsRead, externalapi.ApiTokenScopeAutomationsWrite, externalapi.ApiTokenScopeAutomationsPublish,
	externalapi.ApiTokenScopeEventsWrite,
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
// with internal/accounts and internal/apitokens (the bootstrap token is not used, it
// only addresses the oldest Workspace). Its Inbox deletes the test's own mail on cleanup.
func (e *Env) NewWorkspace(tb testing.TB) *Workspace {
	tb.Helper()
	ctx := tb.Context()
	id := uniq()

	user, err := e.accounts.CreateUser(ctx, "E2E "+id, "owner-"+id+"@e2e.test", "!")
	require.NoError(tb, err)
	ws, err := e.accounts.CreateWorkspace(ctx, user.ID, "e2e "+id)
	require.NoError(tb, err)
	scoped, _, err := e.accounts.Scope(ctx, user.ID, ws.PublicID)
	require.NoError(tb, err)
	minted, err := apitokens.Mint(ctx, scoped, apitokens.Input{Name: "e2e", Scopes: tokenScopes})
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

// Ready is the common setup: an Integration pointing at Mailpit and a verified Sending
// domain, so a Broadcast can leave. Scenarios that need less call the steps themselves.
func (w *Workspace) Ready() *Workspace {
	w.t.Helper()
	w.CreateMailpitIntegration()
	w.AddVerifiedSendingDomain()
	return w
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
	user, err := w.env.accounts.CreateUser(ctx, "E2E co-owner "+id, "co-owner-"+id+"@e2e.test", "!")
	require.NoError(w.t, err)
	// An owner change takes its Workspace feed position inside a transaction.
	err = w.env.bus.WithinScopedTx(ctx, w.scoped, func(ts *ent.Scoped, _ events.Publisher) error {
		_, cerr := ts.Membership().Create().SetUserID(user.ID).SetRole(membership.RoleOwner).Save(ctx)
		return cerr
	})
	require.NoError(w.t, err)
	return w.OwnerMembershipIDs()
}

// OwnerMembershipIDs lists the Membership ids of the Workspace's owners, ascending.
func (w *Workspace) OwnerMembershipIDs() []int64 {
	w.t.Helper()
	ids, err := w.scoped.Membership().Query().
		Where(membership.RoleEQ(membership.RoleOwner)).
		Order(ent.Asc(membership.FieldID)).
		IDs(w.t.Context())
	require.NoError(w.t, err)
	return ids
}

// DemoteConcurrently demotes each Membership to member, all at once, as an owner would,
// and returns the error of each attempt.
func (w *Workspace) DemoteConcurrently(ids ...int64) []error {
	w.t.Helper()
	return w.concurrently(len(ids), func(i int) error {
		_, err := w.env.accounts.ChangeMembershipRole(w.t.Context(), w.scoped, events.Actor{Kind: events.ActorSystem},
			membership.RoleOwner, ids[i], membership.RoleMember)
		return err
	})
}

// RemoveConcurrently removes each Membership, all at once, as an owner would, and returns
// the error of each attempt.
func (w *Workspace) RemoveConcurrently(ids ...int64) []error {
	w.t.Helper()
	return w.concurrently(len(ids), func(i int) error {
		return w.env.accounts.RemoveMembership(w.t.Context(), w.scoped, events.Actor{Kind: events.ActorSystem}, membership.RoleOwner, ids[i])
	})
}

// DemoteAndRemoveConcurrently demotes the first Membership and removes the second at the
// same moment, and returns the error of each attempt.
func (w *Workspace) DemoteAndRemoveConcurrently(demote, remove int64) []error {
	w.t.Helper()
	return w.concurrently(2, func(i int) error {
		if i == 0 {
			_, err := w.env.accounts.ChangeMembershipRole(w.t.Context(), w.scoped, events.Actor{Kind: events.ActorSystem},
				membership.RoleOwner, demote, membership.RoleMember)
			return err
		}
		return w.env.accounts.RemoveMembership(w.t.Context(), w.scoped, events.Actor{Kind: events.ActorSystem}, membership.RoleOwner, remove)
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
