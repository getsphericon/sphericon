package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

// grantSpec is a contract whose grantable scope enums carry grants; Permission and the enums are
// the schemas GrantPermissions reads.
func grantSpec(grantable, send, setup, tokenOnly string) []byte {
	return []byte(`{"components":{"schemas":{
	 "Permission":{"type":"string","enum":["webhooks:manage","integrations:manage"]},
	 "GrantableScope":{"type":"string","enum":["contacts:read","webhooks:read"]` + grantable + `},
	 "SendScope":{"type":"string","enum":["emails:send"]` + send + `},
	 "SetupScope":{"type":"string","enum":["integrations:write"]` + setup + `},
	 "TokenOnlyScope":{"type":"string","enum":["tokens:write"]` + tokenOnly + `}}}}`)
}

const (
	grantableRules = `,"x-grant-permissions":[{"scope":"contacts:read"},{"scope":"webhooks:read","permission":"webhooks:manage"}]`
	sendRules      = `,"x-grant-permissions":[{"scope":"emails:send"}]`
	setupRules     = `,"x-grant-permissions":[{"scope":"integrations:write","permission":"integrations:manage"}]`
)

func TestGenerateGrantsEmitsTheScopesThatNeedAPermission(t *testing.T) {
	src, err := scopegen.GrantPermissions.Generate(grantSpec(grantableRules, sendRules, setupRules, ""))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "Code generated")
	require.Contains(t, out, "package accounts")
	require.Regexp(t, `"webhooks:read":\s+PermWebhooksManage,`, out)
	require.Regexp(t, `"integrations:write":\s+PermIntegrationsManage,`, out)
	require.NotContains(t, out, `"contacts:read"`, "a scope any member may grant has no row")
	require.NotContains(t, out, `"emails:send"`)
}

func TestGenerateGrantsRefusesAGrantableEnumWithoutTheDeclaration(t *testing.T) {
	_, err := scopegen.GrantPermissions.Generate(grantSpec(grantableRules, "", setupRules, ""))

	require.ErrorContains(t, err, "SendScope declares no x-grant-permissions")
}

func TestGenerateGrantsRefusesAScopeLeftUnlisted(t *testing.T) {
	_, err := scopegen.GrantPermissions.Generate(grantSpec(`,"x-grant-permissions":[{"scope":"contacts:read"}]`, sendRules, setupRules, ""))

	require.ErrorContains(t, err, `GrantableScope x-grant-permissions does not list "webhooks:read"`)
}

func TestGenerateGrantsRefusesAScopeOfAnotherEnum(t *testing.T) {
	_, err := scopegen.GrantPermissions.Generate(grantSpec(grantableRules, sendRules, `,"x-grant-permissions":[{"scope":"integrations:write"},{"scope":"tokens:write"}]`, ""))

	require.ErrorContains(t, err, `SetupScope x-grant-permissions lists "tokens:write", which is no SetupScope`)
}

func TestGenerateGrantsRefusesAnUnknownPermission(t *testing.T) {
	_, err := scopegen.GrantPermissions.Generate(grantSpec(grantableRules, sendRules, `,"x-grant-permissions":[{"scope":"integrations:write","permission":"integrations:own"}]`, ""))

	require.ErrorContains(t, err, `the permission "integrations:own", which is no Permission`)
}

func TestGenerateGrantsRefusesADeclarationOnAScopeNoOAuthClientCanBeGranted(t *testing.T) {
	_, err := scopegen.GrantPermissions.Generate(grantSpec(grantableRules, sendRules, setupRules, `,"x-grant-permissions":[{"scope":"tokens:write"}]`))

	require.ErrorContains(t, err, "TokenOnlyScope declares x-grant-permissions")
}
