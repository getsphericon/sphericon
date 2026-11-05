package staff_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/api/staff"
	eestaff "github.com/getsphericon/sphericon/ee/staff"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var (
	primary  = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialPrimaryPrefix, fixtures.ServiceCredentialPrimarySecret)
	readOnly = testhelper.ServiceCredentialValue(fixtures.ServiceCredentialReadOnlyPrefix, fixtures.ServiceCredentialReadOnlySecret)
)

func TestACredentialCannotActOutsideItsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	needsHolds := map[string][]serviceapi.ServiceScope{"NeedsHolds": {serviceapi.ServiceScopeHoldsWrite}, "NeedsNothing": nil}
	security := staff.NewServiceSecurity(env.StaffEnterprise, needsHolds)

	_, err := security.HandleBearerAuth(t.Context(), "NeedsHolds", serviceapi.BearerAuth{Token: readOnly})
	require.ErrorIs(t, err, eestaff.ErrForbidden, "a valid credential without the scope")

	ctx, err := security.HandleBearerAuth(t.Context(), "NeedsHolds", serviceapi.BearerAuth{Token: primary})
	require.NoError(t, err)
	assert.Equal(t, fixtures.ServiceCredentialPrimaryName, staff.CurrentService(ctx).Name)

	_, err = security.HandleBearerAuth(t.Context(), "NeedsNothing", serviceapi.BearerAuth{Token: readOnly})
	require.NoError(t, err, "an operation that needs no scope is open to any valid credential")

	_, err = security.HandleBearerAuth(t.Context(), "NotListed", serviceapi.BearerAuth{Token: primary})
	require.ErrorIs(t, err, eestaff.ErrForbidden, "an operation with no entry is refused to every credential")
}

func TestEveryServiceOperationDeclaresItsScopes(t *testing.T) {
	for _, op := range staff.ServiceOperations() {
		_, declared := staff.ServiceOperationScopes[op]
		assert.True(t, declared, "%s must have an entry in ServiceOperationScopes, even an empty one", op)
	}
}

func TestNoEndpointMintsOrChangesACredential(t *testing.T) {
	surfaces := map[string][]string{
		"service": staff.ServiceOperations(),
		"staff":   methodNames(reflect.TypeFor[staffeeapi.Handler]()),
	}
	for surface, ops := range surfaces {
		require.NotEmpty(t, ops, surface)
		for _, op := range ops {
			for _, banned := range []string{"Credential", "Secret", "Issue", "Rotate", "Revoke"} {
				assert.NotContains(t, op, banned, "the %s surface never mints or changes a credential", surface)
			}
		}
	}
}

func methodNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumMethod())
	for method := range t.Methods() {
		names = append(names, method.Name)
	}
	return names
}
