package site_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// outcome is what the /site server answered a request with: its status and its problem code.
type outcome struct {
	status int
	code   string
}

var (
	// refused is a member without the Permission the operation declares (ADR 0061).
	refused  = outcome{http.StatusForbidden, string(siteapi.ProblemCodePermissionRequired)}
	ok       = outcome{http.StatusOK, ""}
	created  = outcome{http.StatusCreated, ""}
	noRecord = outcome{http.StatusNotFound, ""}
	rejected = outcome{http.StatusUnprocessableEntity, ""}
)

// guarded is one operation of /site that declares a Permission, sent as each Role of Acme.
// Each request is one that passes the gate without changing anything worth keeping (a missing
// row, a mismatched name), so the answer of a Role that holds the Permission is the operation's
// own and the answer of one that does not is the refusal.
type guarded struct {
	permission           siteapi.Permission
	method, path, body   string
	owner, admin, member outcome
}

// acmePath is the /site path of a Workspace operation of Acme.
func acmePath(rest string) string {
	return "/site/workspaces/" + string(fixtures.AcmePublicID) + rest
}

// permissionMatrix holds one operation per Permission, so every cell of the access table of
// ADR 0061 is a request.
var permissionMatrix = []guarded{
	{siteapi.PermissionMembersManage, http.MethodDelete, acmePath("/invitations/999999"), "", noRecord, noRecord, refused},
	{siteapi.PermissionAuditRead, http.MethodGet, acmePath("/audit-entries"), "", ok, ok, refused},
	{siteapi.PermissionAuditConfigure, http.MethodGet, acmePath("/audit-entries/retention"), "", ok, ok, refused},
	{siteapi.PermissionTokensManage, http.MethodDelete, acmePath("/tokens/999999"), "", noRecord, noRecord, refused},
	{siteapi.PermissionIntegrationsManage, http.MethodPost, acmePath("/integrations/999999/archive"), "", noRecord, noRecord, refused},
	{siteapi.PermissionWebhooksManage, http.MethodGet, acmePath("/webhooks"), "", ok, ok, refused},
	{siteapi.PermissionSendingDomainsManage, http.MethodPost, acmePath("/sending-domains/999999/verify"), "", noRecord, noRecord, refused},
	{siteapi.PermissionContactsErase, http.MethodGet, acmePath("/contacts/lookup?email=nobody@example.com"), "", ok, ok, refused},
	{siteapi.PermissionContactsExport, http.MethodPost, acmePath("/contacts/export?id=999999"), "", noRecord, noRecord, refused},
	{siteapi.PermissionSuppressionsManage, http.MethodDelete, acmePath("/suppressions/999999"), "", noRecord, noRecord, refused},
	{siteapi.PermissionSecondFactorManage, http.MethodPost, acmePath("/memberships/999999/reset-second-factor"), "", noRecord, noRecord, refused},
	{siteapi.PermissionWorkspaceUpdate, http.MethodPut, acmePath(""), `{"name":"   "}`, rejected, rejected, refused},
	{siteapi.PermissionWorkspaceTimezone, http.MethodPut, acmePath("/timezone"), `{"timezone":"Mars/Olympus_Mons"}`, rejected, rejected, refused},
	{siteapi.PermissionRevenueConfigure, http.MethodPut, acmePath("/revenue-settings"), `{"reportingCurrency":"XXQ"}`, rejected, rejected, refused},
	{siteapi.PermissionBrandKitManage, http.MethodPost, acmePath("/brand-kit/starter-templates"), "", created, created, refused},
	{siteapi.PermissionJoinDomainsManage, http.MethodGet, acmePath("/join-domains"), "", ok, ok, refused},
	{siteapi.PermissionAccountLinkOpen, http.MethodPost, acmePath("/account-link"), "", ok, refused, refused},
	{siteapi.PermissionWorkspaceDelete, http.MethodPost, acmePath("/delete"), `{"name":"not the name"}`, rejected, refused, refused},
}

func TestTheMatrixHasAnOperationForEveryPermission(t *testing.T) {
	assert.ElementsMatch(t, siteapi.Permission("").AllValues(),
		lo.Map(permissionMatrix, func(g guarded, _ int) siteapi.Permission { return g.permission }))
}

// Every cell of the Permission matrix of ADR 0061, through a real request: a Role that holds the
// Permission reaches the operation, one that does not is answered 403 permission_required.
func TestEveryRoleGetsWhatThePermissionMatrixGrants(t *testing.T) {
	for _, g := range permissionMatrix {
		cells := []struct {
			email string
			want  outcome
		}{
			{fixtures.OwnerJohnEmail, g.owner},
			{fixtures.AdminAliceEmail, g.admin},
			{fixtures.MemberMaryEmail, g.member},
		}
		for _, c := range cells {
			t.Run(string(g.permission)+"/"+c.email, func(t *testing.T) {
				// The Account link is configured, so its operation answers its own.
				env := accountLinkEnv(t)
				assert.Equal(t, c.want, send(t, env, c.email, g.method, g.path, g.body))
			})
		}
	}
}

// An operation that declares no Permission is open to every member, and to no one else.
func TestAnOperationWithoutAPermissionIsOpenToEveryMember(t *testing.T) {
	env := testhelper.Setup(t)
	path := acmePath("/tags")

	assert.Equal(t, ok, send(t, env, fixtures.OwnerJohnEmail, http.MethodGet, path, ""))
	assert.Equal(t, ok, send(t, env, fixtures.AdminAliceEmail, http.MethodGet, path, ""))
	assert.Equal(t, ok, send(t, env, fixtures.MemberMaryEmail, http.MethodGet, path, ""))
	assert.Equal(t, noRecord, send(t, env, fixtures.OutsiderOscarEmail, http.MethodGet, path, ""))
}

// A Workspace that does not exist, or of which the caller is not a member, answers 404 before
// any Permission is checked, so its existence does not leak.
func TestAnUnknownOrForeignWorkspaceIsNotFoundBeforeAPermissionIsChecked(t *testing.T) {
	env := testhelper.Setup(t)

	assert.Equal(t, noRecord, send(t, env, fixtures.OutsiderOscarEmail, http.MethodGet, acmePath("/audit-entries"), ""))
	assert.Equal(t, noRecord, send(t, env, fixtures.MemberMaryEmail, http.MethodGet, "/site/workspaces/NoSuchWs/audit-entries", ""))
}

// send makes the request as the fixture user with the given login email and reads the answer.
func send(t *testing.T, env *testhelper.TestEnv, email, method, path, body string) outcome {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(testhelper.RequestCookie("JWT", env.SiteToken(t, email, nil)))
	env.Server.ServeHTTP(rec, req)
	var answer any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &answer), rec.Body.String())
	return outcome{status: rec.Code, code: codeOf(answer)}
}

// codeOf is the code of a problem answer, empty for any other body (an object without one, a
// list).
func codeOf(answer any) string {
	problem, _ := answer.(map[string]any)
	code, _ := problem["code"].(string)
	return code
}
