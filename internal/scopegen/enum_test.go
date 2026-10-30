package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

func TestGeneratePermissionsDeclaresTheTypeAndAConstantPerValue(t *testing.T) {
	src, err := scopegen.Permissions.Generate([]byte(`{"components":{"schemas":{
	 "Permission":{"type":"string","enum":["members:manage","sending_domains:manage"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package accounts")
	require.Contains(t, out, "type Permission string")
	require.Regexp(t, `PermMembersManage\s+Permission = "members:manage"`, out)
	require.Regexp(t, `PermSendingDomainsManage\s+Permission = "sending_domains:manage"`, out)
}

func TestGenerateTokenScopesListsTheVocabularyInTheContractsOrder(t *testing.T) {
	src, err := scopegen.TokenScopes.Generate([]byte(`{"components":{"schemas":{
	 "ApiTokenScope":{"type":"string","enum":["mcp:send","custom_fields:read"]}}}}`))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package apitokens")
	require.Regexp(t, `ScopeMcpSend\s+Scope = "mcp:send"`, out)
	require.Regexp(t, `var Scopes = \[\]Scope\{\s+ScopeMcpSend,\s+ScopeCustomFieldsRead,\s+\}`, out)
}

func TestGenerateAnEnumFileRefusesADocumentWithoutTheEnum(t *testing.T) {
	_, err := scopegen.TokenScopes.Generate([]byte(`{"components":{"schemas":{}}}`))

	require.ErrorContains(t, err, "the document has no ApiTokenScope enum")
}
