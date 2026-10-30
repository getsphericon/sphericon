package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

const externalSpec = `{"paths":{
 "/contacts":{"get":{"operationId":"Contacts_list","x-required-scopes":["contacts:read"]}},
 "/automations/dry-run-http-step":{"post":{"operationId":"Automations_dryRunHttpStep","x-required-scopes":["automations:write","contacts:read"]}},
 "/auth/me":{"get":{"operationId":"Auth_meGet","x-required-scopes":[]}}},
 "components":{"schemas":{"ApiTokenScope":{"type":"string","enum":["contacts:read","automations:write"]},
  "Status":{"type":"integer","enum":[1,2]}}}}`

func TestGenerateExternalEmitsOneRowPerOperationWithItsScopes(t *testing.T) {
	src, err := scopegen.ExternalScopes.Generate([]byte(externalSpec))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "Code generated")
	require.Contains(t, out, "package auth")
	require.Regexp(t, `ContactsListOperation:\s+\{apitokens.ScopeContactsRead\},`, out)
	require.Regexp(t, `AutomationsDryRunHttpStepOperation:\s+\{apitokens.ScopeAutomationsWrite, apitokens.ScopeContactsRead\},`, out)
	require.Regexp(t, `AuthMeGetOperation:\s+nil,`, out)
}

func TestGenerateExternalRefusesAnOperationThatDeclaresNoScopes(t *testing.T) {
	_, err := scopegen.ExternalScopes.Generate([]byte(`{"paths":{"/x":{"get":{"operationId":"Contacts_list"}}},
	 "components":{"schemas":{"ApiTokenScope":{"type":"string","enum":["contacts:read"]}}}}`))

	require.ErrorContains(t, err, "(Contacts_list) declares no x-required-scopes")
}

func TestGenerateExternalRefusesAScopeThatIsNoApiTokenScope(t *testing.T) {
	_, err := scopegen.ExternalScopes.Generate([]byte(`{"paths":{"/x":{"get":{"operationId":"Contacts_list","x-required-scopes":["contacts:peek"]}}},
	 "components":{"schemas":{"ApiTokenScope":{"type":"string","enum":["contacts:read"]}}}}`))

	require.ErrorContains(t, err, `requires "contacts:peek", which is no ApiTokenScope`)
}

const externalPermissionSpec = `{"paths":{
 "/contacts/export":{"get":{"operationId":"Contacts_export","x-required-scopes":["contacts:read"],"x-required-permissions":["contacts:export","contacts:erase"]}},
 "/suppressions":{"post":{"operationId":"Suppressions_create","x-required-scopes":["contacts:write"],"x-required-permissions":["suppressions:manage"]}},
 "/contacts":{"get":{"operationId":"Contacts_list","x-required-scopes":["contacts:read"]}}},
 "components":{"schemas":{"Permission":{"type":"string","enum":["contacts:export","contacts:erase","suppressions:manage"]}}}}`

func TestGenerateExternalPermissionsEmitsTheOperationsThatDeclareThem(t *testing.T) {
	src, err := scopegen.ExternalPermissions.Generate([]byte(externalPermissionSpec))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package auth")
	require.Regexp(t, `ContactsExportOperation:\s+\{accounts.PermContactsExport, accounts.PermContactsErase\},`, out)
	require.Regexp(t, `SuppressionsCreateOperation:\s+\{accounts.PermSuppressionsManage\},`, out)
	require.NotContains(t, out, "ContactsListOperation", "an operation that declares no Permission has no entry")
}

func TestGenerateExternalPermissionsRefusesAnUnknownPermission(t *testing.T) {
	_, err := scopegen.ExternalPermissions.Generate([]byte(`{"paths":{"/x":{"get":{"operationId":"Contacts_export","x-required-permissions":["contacts:peek"]}}},
	 "components":{"schemas":{"Permission":{"type":"string","enum":["contacts:export"]}}}}`))

	require.ErrorContains(t, err, `(Contacts_export) requires "contacts:peek", which is no Permission`)
}
