package site_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// entriesNamed delivers the outbox to the EE subscriber and returns the entries of
// one action as the Workspace's owner sees them, newest first.
func entriesNamed(t *testing.T, env *testhelper.TestEnv, ownerEmail string, workspaceID siteapi.WorkspaceId, action string) []siteapi.SiteAuditEntryResource {
	t.Helper()
	env.DeliverToEE(t)
	var out []siteapi.SiteAuditEntryResource
	items := auditPage(t, env.SiteActor(t, ownerEmail), workspaceID).Items
	for i := range items {
		if string(items[i].Action) == action {
			out = append(out, items[i])
		}
	}
	return out
}

func TestAuditInvitationCreateAndRevokeAreRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := owner.SiteInvitationsCreate(ctx,
		&siteapi.SiteCreateInvitationInput{Email: "newbie@acme.test", Role: siteapi.SiteInvitableRoleAdmin},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteCreateInvitationResponse)
	require.Truef(t, ok, "got %T", res)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionInvitationCreate)
	require.Len(t, got, 1)
	e := got[0]
	assert.Equal(t, siteapi.SiteAuditActorKindUser, e.Actor.Kind)
	assert.Equal(t, "John", e.Actor.Name.Value)
	assert.Equal(t, "invitation", e.Target.Type)
	assert.Equal(t, strconv.FormatInt(int64(created.Resource.ID), 10), e.Target.ID.Value)
	assert.Equal(t, "newbie@acme.test", e.Target.Name.Value)
	diff, err := json.Marshal(e.Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":{"to":"admin"}}`, string(diff))
	assert.NotContains(t, string(diff), created.InviteUrl, "the link is a credential and never logged")

	del, err := owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: created.Resource.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteInvitationsDeleteNoContent{}, del)

	revoked := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionInvitationRevoke)
	require.Len(t, revoked, 1)
	assert.Equal(t, "newbie@acme.test", revoked[0].Target.Name.Value)
	assert.Equal(t, strconv.FormatInt(int64(created.Resource.ID), 10), revoked[0].Target.ID.Value)
}

func TestAuditInvitationAcceptIsAttributedToTheNewMember(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(context.Background(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Ivy"), Password: siteapi.NewOptString("ivy-password-1")},
		siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionInvitationAccept)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
	assert.Equal(t, "Ivy", got[0].Actor.Name.Value)
	assert.Equal(t, "invited@acme.test", got[0].Target.Name.Value)
	assert.Empty(t, entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionInvitationAccept), "only the inviting Workspace")
}

func TestAuditLoginDuplicatesIntoEachOfTheUsersWorkspacesOnly(t *testing.T) {
	env := testhelper.Setup(t)

	// Mary belongs to Acme and Globex; John to Acme only.
	require.Equal(t, 200, loginStatus(t, env, fixtures.MemberMaryEmail, fixtures.MemberMaryPassword))
	require.Equal(t, 401, loginStatus(t, env, fixtures.OwnerJohnEmail, "wrong"), "a failed login is not recorded")

	acme := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserLogin)
	globex := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserLogin)
	require.Len(t, acme, 1)
	require.Len(t, globex, 1)
	for _, e := range []siteapi.SiteAuditEntryResource{acme[0], globex[0]} {
		assert.Equal(t, siteapi.SiteAuditActorKindUser, e.Actor.Kind)
		assert.Equal(t, "Mary", e.Actor.Name.Value)
	}

	// John's login lands in Acme, never in Globex.
	signUpAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1") // user.email_verify
	require.Equal(t, 200, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword))
	assert.Len(t, entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserLogin), 2)
	assert.Len(t, entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserLogin), 1)
}

func TestAuditPasswordChangeDuplicatesIntoEachOfTheUsersWorkspaces(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	res, err := mary.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{
		CurrentPassword: siteapi.NewOptString(fixtures.MemberMaryPassword),
		NewPassword:     siteapi.NewOptString("brand-new-pass-1"),
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteUserResourceHeaders{}, res)

	acme := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserPasswordChange)
	globex := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserPasswordChange)
	require.Len(t, acme, 1)
	require.Len(t, globex, 1)
	diff, err := json.Marshal(acme[0].Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"password":"changed"}`, string(diff), "a secret records only that it changed")
	assert.NotContains(t, string(diff), "brand-new-pass-1")

	// A name-only edit is not a password change.
	_, err = mary.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{Name: siteapi.NewOptString("Maria")})
	require.NoError(t, err)
	assert.Len(t, entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserPasswordChange), 1)
}

func TestAuditPasswordResetIsRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteAnonymous(t)
	_, err := c.SiteAuthForgotPassword(ctx, &siteapi.SiteForgotPasswordInput{Email: fixtures.MemberMaryEmail})
	require.NoError(t, err)
	_, body := lastSystemEmail(t, env)
	res, err := c.SiteAuthResetPassword(ctx, &siteapi.SiteResetPasswordInput{Token: tokenFromEmail(t, body), Password: "reset-pass-123"})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteAuthResetPasswordOK{}, res)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionUserPasswordChange)
	require.Len(t, got, 1)
	assert.Equal(t, "Mary", got[0].Actor.Name.Value)
}

func TestAuditStaffSuspensionShowsOnlySphericonStaff(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), "abuse report")
	require.NoError(t, err)
	_, err = suspension.UnsuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"))
	require.NoError(t, err)

	for _, action := range []string{events.ActionWorkspaceSuspend, events.ActionWorkspaceUnsuspend} {
		got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, action)
		require.Lenf(t, got, 1, action)
		assert.Equal(t, siteapi.SiteAuditActorKindStaff, got[0].Actor.Kind)
		assert.Equal(t, "Sphericon staff", got[0].Actor.Name.Value)
		assert.False(t, got[0].Actor.ID.IsSet() && got[0].Actor.ID.Value != "", "the StaffMember's identity is never exposed")
		diff, err := json.Marshal(got[0].Diff.Value)
		require.NoError(t, err)
		assert.NotContains(t, string(diff), "op-42")
		assert.NotContains(t, got[0].Target.Name.Value, "op-42")
		assert.Equal(t, "workspace", got[0].Target.Type)
		assert.Equal(t, strconv.Itoa(fixtures.AcmeID), got[0].Target.ID.Value)
	}
}

func TestAuditAutomaticSuspensionIsAttributedToSystem(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.SuspendWorkspace(context.Background(), env.Bus, fixtures.AcmeID, suspension.System, "complaint rate")
	require.NoError(t, err)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceSuspend)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindSystem, got[0].Actor.Kind)
}

func TestAuditCLISuspensionShowsSphericonStaff(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := suspension.SuspendWorkspace(context.Background(), env.Bus, fixtures.AcmeID, suspension.CLI, "abuse")
	require.NoError(t, err)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceSuspend)
	require.Len(t, got, 1)
	assert.Equal(t, siteapi.SiteAuditActorKindStaff, got[0].Actor.Kind)
	assert.Equal(t, "Sphericon staff", got[0].Actor.Name.Value)
	assert.False(t, got[0].Actor.ID.IsSet() && got[0].Actor.ID.Value != "")
}

func TestAuditSuspensionNoOpRecordsNothing(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.System, "first")
	require.NoError(t, err)
	changed, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), "second")
	require.NoError(t, err)
	require.False(t, changed)

	assert.Len(t, entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceSuspend), 1)
}

// Every explicit path is driven here and the actions that reached the log are compared
// with events.ExplicitAuditActions, so a new explicit path cannot be forgotten: either
// it is added to the list (and driven below) or this test fails. An explicit entry is
// recorded one way, events.RecordAudit in the transaction of the change, and the call-site
// scan catches such a call in a file this list does not know about.
func TestExplicitAuditPathsAreAllListed(t *testing.T) {
	env, fake := signinEnv(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	promoteMary(t, env)                                                             // membership.update
	_, err := owner.SiteMembershipsDelete(ctx, siteapi.SiteMembershipsDeleteParams{ // membership.delete
		WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeAdminMembershipID)})
	require.NoError(t, err)
	created, err := owner.SiteInvitationsCreate(ctx, // invitation.create
		&siteapi.SiteCreateInvitationInput{Email: "x@acme.test", Role: siteapi.SiteInvitableRoleMember},
		siteapi.SiteInvitationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = owner.SiteInvitationsDelete(ctx, siteapi.SiteInvitationsDeleteParams{ // invitation.revoke
		WorkspaceId: fixtures.AcmePublicID, ID: created.(*siteapi.SiteCreateInvitationResponse).Resource.ID})
	require.NoError(t, err)
	_, err = env.SiteAnonymous(t).SitePublicInvitationsAccept(ctx, // invitation.accept
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Ivy"), Password: siteapi.NewOptString("ivy-password-1")},
		siteapi.SitePublicInvitationsAcceptParams{Token: "inv_fixture_token_acme"})
	require.NoError(t, err)
	signUpAndVerify(t, env, "Grace Hopper", "grace@example.com", "grace-password-1")                // user.email_verify
	enableAcmeJoin(t, env)                                                                          // workspace.update (the domain join switch)
	signUpAndVerify(t, env, "Joiner", "joiner@acme.com", "joiner-password-1")                       // membership.domain_join
	require.Equal(t, 200, loginStatus(t, env, fixtures.OwnerJohnEmail, fixtures.OwnerJohnPassword)) // user.login
	_, err = owner.SiteUserUpdateMe(ctx, &siteapi.SiteUpdateMeInput{                                // user.password_change
		CurrentPassword: siteapi.NewOptString(fixtures.OwnerJohnPassword), NewPassword: siteapi.NewOptString("another-pass-1")})
	require.NoError(t, err)
	started, err := owner.SiteSecondFactorStartEnrollment(ctx, &siteapi.SiteSecondFactorStartInput{CurrentPassword: siteapi.NewOptString("another-pass-1")})
	require.NoError(t, err)
	code, err := totp.GenerateCode(started.(*siteapi.SiteSecondFactorEnrollment).Secret, time.Now())
	require.NoError(t, err)
	_, err = owner.SiteSecondFactorConfirmEnrollment(ctx, &siteapi.SiteSecondFactorConfirmInput{CurrentPassword: siteapi.NewOptString("another-pass-1"), Code: code}) // user.second_factor_enroll
	require.NoError(t, err)
	regenerated, err := owner.SiteSecondFactorRegenerateRecoveryCodes(ctx, // user.recovery_codes_regenerate
		&siteapi.SiteRecoveryCodesInput{CurrentPassword: siteapi.NewOptString("another-pass-1")})
	require.NoError(t, err)
	disabled, err := owner.SiteSecondFactorDisable(ctx, &siteapi.SiteSecondFactorDisableInput{ // user.recovery_code_use, user.second_factor_disable
		CurrentPassword: siteapi.NewOptString("another-pass-1"), Code: regenerated.(*siteapi.SiteRecoveryCodesHeaders).Response.Codes[0]})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteSecondFactorDisableNoContent{}, disabled)
	_, err = env.SiteActor(t, fixtures.OwnerJaneEmail).SiteMembershipsResetSecondFactor(ctx, // user.second_factor_reset
		siteapi.SiteMembershipsResetSecondFactorParams{WorkspaceId: fixtures.GlobexPublicID, ID: idStr(fixtures.GlobexSamMembershipID)})
	require.NoError(t, err)
	_, err = owner.SiteWorkspacesCreate(ctx, &siteapi.SiteCreateWorkspaceInput{Name: "Acme Three"}) // workspace.create
	require.NoError(t, err)
	_, err = owner.SiteWorkspacesUpdate(ctx, &siteapi.SiteUpdateWorkspaceInput{Name: "Acme Two"}, // workspace.update
		siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = owner.SiteWorkspacesDelete(ctx, &siteapi.SiteDeleteWorkspaceInput{Name: "Acme Two"}, // workspace.delete
		siteapi.SiteWorkspacesDeleteParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = owner.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{ID: fixtures.AcmePublicID}) // workspace.restore
	require.NoError(t, err)
	_, err = suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), "abuse") // workspace.suspend
	require.NoError(t, err)
	_, err = suspension.UnsuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42")) // workspace.unsuspend
	require.NoError(t, err)
	_, err = suspension.SetAbuseExempt(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), true) // workspace.abuse_exempt
	require.NoError(t, err)
	_, err = suspension.SetAbuseExempt(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), false) // workspace.abuse_unexempt
	require.NoError(t, err)
	_, err = suspension.SetSendCap(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), new(500)) // workspace.send_cap_change
	require.NoError(t, err)

	_, err = env.ExternalAnchor(t).ContactsBatchUpsert(ctx, &externalapi.UpsertContactsInput{ // contact.import
		Contacts: []externalapi.UpsertContactInput{{Email: externalapi.NewOptNilEmailAddress("explicit.import@example.org")}}})
	require.NoError(t, err)

	_, err = owner.SiteAuditExport(ctx, siteapi.SiteAuditExportParams{WorkspaceId: fixtures.AcmePublicID}) // audit_log.export
	require.NoError(t, err)

	_, err = owner.SiteWebhooksTest(ctx, &siteapi.TestWebhookInput{EventType: "contact.created"}, // webhook_endpoint.test
		siteapi.SiteWebhooksTestParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)})
	require.NoError(t, err)

	b := signedIn(t, env, fake, mary)                                                                                   // user.login by a provider
	link(t, fake, b, maryAtWork)                                                                                        // user.identity_link
	require.Equal(t, http.StatusNoContent, b.Delete(t, identitiesPath+"/"+listIdentities(t, b).Items[0].ID).StatusCode) // user.identity_unlink

	_, err = owner.SiteUserSignOutEverywhere(ctx) // user.sign_out_everywhere
	require.NoError(t, err)

	var seen []string
	for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
		seen = append(seen, ev.(*events.AuditEntry).Action)
	}
	slices.Sort(seen)
	seen = slices.Compact(seen)
	want := slices.Clone(events.ExplicitAuditActions)
	slices.Sort(want)
	assert.Equal(t, want, seen)

	assert.Equal(t, []string{"ee/audit/export.go", "internal/accounts/accounts.go", "internal/accounts/join.go", "internal/accounts/roles.go", "internal/contacts/contacts.go", "internal/suspension/suspension.go", "internal/webhooks/webhooks.go", "internal/workspacedeletion/workspacedeletion.go"},
		recordAuditCallSites(t), "a new explicit RecordAudit call site: list its actions and drive them above")
}

func recordAuditCallSites(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..", "..")
	var sites []string
	for _, dir := range []string{"internal", "ee", "cmd"} {
		require.NoError(t, filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			// Package events calls RecordAudit unqualified only on the automatic path (the
			// generated scoped-client wrappers publish through it), never an explicit entry.
			if strings.Contains(string(src), "events.RecordAudit(") {
				sites = append(sites, rel)
			}
			return nil
		}))
	}
	slices.Sort(sites)
	return sites
}

// A Staff member's id cannot be probed: filtering by it matches nothing, and with the
// staff member kind the id filter is ignored, on the list and on the export alike.
func TestAuditStaffIdCannotBeProbed(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_, err := suspension.SuspendWorkspace(ctx, env.Bus, fixtures.AcmeID, suspension.StaffMember("op-42"), "abuse report")
	require.NoError(t, err)
	env.DeliverToEE(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list := func(p siteapi.SiteAuditListParams) int {
		p.WorkspaceId = fixtures.AcmePublicID
		res, err := owner.SiteAuditList(ctx, p)
		require.NoError(t, err)
		return len(res.(*siteapi.SiteAuditEntryList).Items)
	}
	export := func(p siteapi.SiteAuditExportParams) int {
		p.WorkspaceId = fixtures.AcmePublicID
		return len(auditCSV(t, mustExport(ctx, t, owner, p))) - 1
	}
	staff := siteapi.NewOptSiteAuditActorKind(siteapi.SiteAuditActorKindStaff)
	probe, other := siteapi.NewOptString("op-42"), siteapi.NewOptString("someone-else")

	assert.Zero(t, list(siteapi.SiteAuditListParams{ActorId: probe}), "the real id matches nothing without a kind")
	assert.Zero(t, export(siteapi.SiteAuditExportParams{ActorId: probe}))
	assert.Equal(t, 1, list(siteapi.SiteAuditListParams{ActorKind: staff, ActorId: probe}))
	assert.Equal(t, 1, list(siteapi.SiteAuditListParams{ActorKind: staff, ActorId: other}), "a wrong id changes nothing: the id is ignored")
	assert.Equal(t, 1, export(siteapi.SiteAuditExportParams{ActorKind: staff, ActorId: other}))
}
