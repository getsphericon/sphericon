package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

const serviceSpec = `{"paths":{
 "/usage":{"get":{"operationId":"ServiceUsage_list","x-required-scopes":["usage:read"]}},
 "/holds":{"put":{"operationId":"ServiceHolds_set","x-required-scopes":["holds:write","usage:read"]}},
 "/identity":{"get":{"operationId":"ServiceIdentity_get","x-required-scopes":[]}}}}`

func TestGenerateEmitsOneRowPerOperationWithItsScopes(t *testing.T) {
	src, err := scopegen.ServiceScopes.Generate([]byte(serviceSpec))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "Code generated")
	require.Regexp(t, `ServiceUsageListOperation:\s+\{serviceapi.ServiceScopeUsageRead\},`, out)
	require.Regexp(t, `ServiceHoldsSetOperation:\s+\{serviceapi.ServiceScopeHoldsWrite, serviceapi.ServiceScopeUsageRead\},`, out)
	require.Regexp(t, `ServiceIdentityGetOperation:\s+nil,`, out)
}

func TestGenerateRefusesAnOperationThatDeclaresNoScopes(t *testing.T) {
	_, err := scopegen.ServiceScopes.Generate([]byte(`{"paths":{"/x":{"get":{"operationId":"Service_list"}}}}`))

	require.ErrorContains(t, err, "Service_list")
}

const siteSpec = `{"paths":{
 "/workspaces/{workspaceId}/audit-entries":{"get":{"operationId":"SiteAudit_list","x-required-permissions":["audit:read"]}},
 "/workspaces/{workspaceId}/tags":{"get":{"operationId":"SiteTags_list","x-required-permissions":[]}},
 "/workspaces/{workspaceId}/second-factor-requirement":{"delete":{"operationId":"SiteWorkspaces_liftSecondFactorRequirement","x-required-permissions":["second_factor:manage"],"x-allowed-while-withheld":true}},
 "/auth/login":{"post":{"operationId":"SiteAuth_login"}}}}`

func TestGenerateEmitsTheAccessOfEverySiteOperation(t *testing.T) {
	src, err := scopegen.SitePermissions.Generate([]byte(siteSpec))
	require.NoError(t, err)

	out := string(src)
	require.Regexp(t, `SiteAuditListOperation:\s+\{workspace: true, permissions: \[\]accounts.Permission\{accounts.PermAuditRead\}\},`, out)
	require.Regexp(t, `SiteTagsListOperation:\s+\{workspace: true\},`, out)
	require.Regexp(t, `SiteWorkspacesLiftSecondFactorRequirementOperation:\s+\{workspace: true, permissions: \[\]accounts.Permission\{accounts.PermSecondFactorManage\}, whileWithheld: true\},`, out)
	require.Regexp(t, `SiteAuthLoginOperation:\s+\{\},`, out)
}

func TestGenerateRefusesAWorkspaceOperationThatDeclaresNoPermissions(t *testing.T) {
	_, err := scopegen.SitePermissions.Generate([]byte(`{"paths":{"/workspaces/{workspaceId}/tags":{"get":{"operationId":"SiteTags_list"}}}}`))

	require.ErrorContains(t, err, "SiteTags_list")
	require.ErrorContains(t, err, "declares no x-required-permissions")
}

func TestGenerateRefusesPermissionsOnAnOperationWithoutAWorkspace(t *testing.T) {
	_, err := scopegen.SitePermissions.Generate([]byte(`{"paths":{"/me":{"get":{"operationId":"SiteUser_getMe","x-required-permissions":[]}}}}`))

	require.ErrorContains(t, err, "SiteUser_getMe")
	require.ErrorContains(t, err, "has no workspaceId")
}

func TestGenerateRefusesTheWithheldMarkOnAnOperationWithoutAWorkspace(t *testing.T) {
	_, err := scopegen.SitePermissions.Generate([]byte(`{"paths":{"/me":{"get":{"operationId":"SiteUser_getMe","x-allowed-while-withheld":true}}}}`))

	require.ErrorContains(t, err, "SiteUser_getMe")
	require.ErrorContains(t, err, "x-allowed-while-withheld")
}

func TestGenerateMarksTheOperationsAUserWithAnAcceptancePendingMayCall(t *testing.T) {
	src, err := scopegen.SitePermissions.Generate([]byte(`{"paths":{
 "/me":{"get":{"operationId":"SiteUser_getMe","x-allowed-while-acceptance-pending":true}},
 "/me/email-change":{"post":{"operationId":"SiteUser_emailChange"}},
 "/workspaces/{workspaceId}/tags":{"get":{"operationId":"SiteTags_list","x-required-permissions":[],"x-allowed-while-acceptance-pending":true}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Regexp(t, `SiteUserGetMeOperation:\s+\{whileAcceptancePending: true\},`, out)
	require.Regexp(t, `SiteUserEmailChangeOperation:\s+\{\},`, out)
	require.Regexp(t, `SiteTagsListOperation:\s+\{workspace: true, whileAcceptancePending: true\},`, out)
}

// An anonymous operation carries no session, so the gate never refuses it; the mark there only
// declares in the contract that a gated User may call it.
func TestGenerateAcceptsTheAcceptancePendingMarkOnAnAnonymousOperation(t *testing.T) {
	src, err := scopegen.SitePermissions.Generate([]byte(`{"paths":{
 "/auth/logout":{"post":{"operationId":"SiteAuth_logout","security":[{}],"x-allowed-while-acceptance-pending":true}}}}`))
	require.NoError(t, err)

	require.Regexp(t, `SiteAuthLogoutOperation:\s+\{whileAcceptancePending: true\},`, string(src))
}

// A mark is true or absent: false says nothing an absent mark does not, so either mark refuses it.
func TestGenerateRefusesAMalformedMark(t *testing.T) {
	cases := map[string]struct{ spec, mark string }{
		"acceptance pending, not a boolean": {
			`{"paths":{"/me":{"get":{"operationId":"SiteUser_getMe","x-allowed-while-acceptance-pending":"yes"}}}}`,
			"x-allowed-while-acceptance-pending",
		},
		"acceptance pending, false": {
			`{"paths":{"/me":{"get":{"operationId":"SiteUser_getMe","x-allowed-while-acceptance-pending":false}}}}`,
			"x-allowed-while-acceptance-pending",
		},
		"withheld, false": {
			`{"paths":{"/workspaces/{workspaceId}/tags":{"get":{"operationId":"SiteUser_getMe","x-required-permissions":[],"x-allowed-while-withheld":false}}}}`,
			"x-allowed-while-withheld",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := scopegen.SitePermissions.Generate([]byte(c.spec))

			require.ErrorContains(t, err, "SiteUser_getMe")
			require.ErrorContains(t, err, c.mark)
		})
	}
}
