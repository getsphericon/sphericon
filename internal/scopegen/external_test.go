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
	require.Regexp(t, `ContactsListOperation:\s+\{externalapi.ApiTokenScopeContactsRead\},`, out)
	require.Regexp(t, `AutomationsDryRunHttpStepOperation:\s+\{externalapi.ApiTokenScopeAutomationsWrite, externalapi.ApiTokenScopeContactsRead\},`, out)
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
