package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

const spec = `{"paths":{
 "/usage":{"get":{"operationId":"ServiceUsage_list","x-required-scopes":["usage:read"]}},
 "/holds":{"put":{"operationId":"ServiceHolds_set","x-required-scopes":["holds:write","usage:read"]}},
 "/identity":{"get":{"operationId":"ServiceIdentity_get","x-required-scopes":[]}}}}`

func TestGenerateEmitsOneRowPerOperationWithItsScopes(t *testing.T) {
	src, err := scopegen.Generate([]byte(spec))
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "Code generated")
	require.Regexp(t, `ServiceUsageListOperation:\s+\{serviceapi.ServiceScopeUsageRead\},`, out)
	require.Regexp(t, `ServiceHoldsSetOperation:\s+\{serviceapi.ServiceScopeHoldsWrite, serviceapi.ServiceScopeUsageRead\},`, out)
	require.Regexp(t, `ServiceIdentityGetOperation:\s+nil,`, out)
}

func TestGenerateRefusesAnOperationThatDeclaresNoScopes(t *testing.T) {
	_, err := scopegen.Generate([]byte(`{"paths":{"/x":{"get":{"operationId":"Service_list"}}}}`))

	require.ErrorContains(t, err, "Service_list")
}
