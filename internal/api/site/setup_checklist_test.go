package site_test

import (
	"context"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	todo    = siteapi.SiteSetupStepStatusTodo
	pending = siteapi.SiteSetupStepStatusPending
	done    = siteapi.SiteSetupStepStatusDone
)

func setupChecklist(t *testing.T, email string, workspaceID siteapi.WorkspaceId) *siteapi.SiteSetupProgress {
	t.Helper()
	c := testhelper.Setup(t).SiteActor(t, email)
	res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	out, ok := res.(*siteapi.SiteSetupProgress)
	require.Truef(t, ok, "got %T", res)
	return out
}

func TestSiteSetupChecklistRequiresAuth(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestSiteSetupChecklistHidesAForeignWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.ProblemDetails{}, res)
}

func TestSiteSetupChecklistTestInboxStepIsDoneByCreatingOneAndNeverMakesItDefault(t *testing.T) {
	c := testhelper.Setup(t).SiteActor(t, fixtures.OwnerSallyEmail)
	read := func() *siteapi.SiteSetupProgress {
		res, err := c.SiteSetupChecklistGet(context.Background(), siteapi.SiteSetupChecklistGetParams{WorkspaceId: fixtures.HooliPublicID})
		require.NoError(t, err)
		out, ok := res.(*siteapi.SiteSetupProgress)
		require.Truef(t, ok, "got %T", res)
		return out
	}
	require.Equal(t, todo, read().Steps.TestInbox)

	res, err := c.SiteIntegrationsCreate(context.Background(), &siteapi.SiteCreateIntegrationInput{
		Name: "Test inbox", IsDefault: siteapi.NewOptBool(false), Config: inboxInput(),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.HooliPublicID})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteIntegrationResource)
	require.Truef(t, ok, "got %T", res)
	assert.False(t, created.IsDefault)

	got := read()
	assert.Equal(t, siteapi.SiteSetupSteps{Integration: done, TestInbox: done, SendingDomain: todo, Contact: todo, SentEmail: todo}, got.Steps)
	assert.False(t, got.Complete)
}

func TestSiteSetupChecklistStates(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		workspaceID siteapi.WorkspaceId
		want        siteapi.SiteSetupSteps
		complete    bool
	}{
		{"a Workspace with nothing is all todo; a skipped message is not a sent email", fixtures.OwnerSallyEmail, fixtures.HooliPublicID,
			siteapi.SiteSetupSteps{Integration: todo, TestInbox: todo, SendingDomain: todo, Contact: todo, SentEmail: todo}, false},
		{"a domain waiting for DNS is pending and blocks nothing; a live Test inbox is done", fixtures.OwnerJaneEmail, fixtures.GlobexPublicID,
			siteapi.SiteSetupSteps{Integration: done, TestInbox: done, SendingDomain: pending, Contact: done, SentEmail: done}, false},
		{"a verified domain, Contacts and Broadcasts without an Integration", fixtures.InitechOwnerOlgaEmail, fixtures.InitechPublicID,
			siteapi.SiteSetupSteps{Integration: todo, TestInbox: todo, SendingDomain: done, Contact: done, SentEmail: done}, false},
		{"a Transactional-only Workspace completes the last step", fixtures.KramericaOwnerNewmanEmail, fixtures.KramericaPublicID,
			siteapi.SiteSetupSteps{Integration: todo, TestInbox: todo, SendingDomain: done, Contact: todo, SentEmail: done}, false},
		{"a fully set up Workspace is complete; its archived Test inbox does not count", fixtures.OwnerJohnEmail, fixtures.AcmePublicID,
			siteapi.SiteSetupSteps{Integration: done, TestInbox: todo, SendingDomain: done, Contact: done, SentEmail: done}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := setupChecklist(t, tc.owner, tc.workspaceID)
			assert.Equal(t, tc.want, got.Steps)
			assert.Equal(t, tc.complete, got.Complete)
		})
	}
}
