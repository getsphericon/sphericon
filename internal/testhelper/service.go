package testhelper

import (
	"context"
	"testing"

	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ee/staff"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/credentials"
)

const serviceBase = "http://local/service"

// ServiceClient returns a service-endpoints client presenting value as its bearer
// credential, served by the internal listener's handler (env.Internal), the only place
// the service endpoints exist. Build the value of a fixture credential with
// ServiceCredentialValue, or issue one with IssueServiceCredential.
func (env *TestEnv) ServiceClient(t *testing.T, value string) *serviceapi.Client {
	t.Helper()
	require.NotNil(t, env.Internal, "the instance serves no service endpoints (no staff license)")
	return env.serviceClientOn(t, injectClient{handler: env.Internal}, value)
}

// ServiceClientOnPublicPort is ServiceClient aimed at the public server instead, where
// the service endpoints must not exist.
func (env *TestEnv) ServiceClientOnPublicPort(t *testing.T, value string) *serviceapi.Client {
	t.Helper()
	return env.serviceClientOn(t, env.Transport(nil), value)
}

func (env *TestEnv) serviceClientOn(t *testing.T, transport ht.Client, value string) *serviceapi.Client {
	t.Helper()
	c, err := serviceapi.NewClient(serviceBase, serviceBearer{value}, serviceapi.WithClient(transport))
	require.NoError(t, err)
	return c
}

// IssueServiceCredential issues a Service credential through the same module the
// command line uses and returns the value to present: the issuing helper of the tests.
// The credential belongs to the service called name and carries exactly scopes.
func (env *TestEnv) IssueServiceCredential(t *testing.T, name string, scopes ...staff.Scope) string {
	t.Helper()
	issued, err := env.StaffMembers.IssueServiceCredential(t.Context(), staff.IssueInput{Name: name, Scopes: scopes})
	require.NoError(t, err)
	return issued.Value
}

// ServiceCredentialValue is the value a service presents for a fixture credential, from
// the fixture catalog's prefix and secret (fixtures.ServiceCredentialPrimaryPrefix and
// fixtures.ServiceCredentialPrimarySecret, say).
func ServiceCredentialValue(prefix, secret string) string {
	return credentials.ServiceCredentialValue(prefix, secret)
}

type serviceBearer struct{ value string }

func (s serviceBearer) BearerAuth(context.Context, serviceapi.OperationName) (serviceapi.BearerAuth, error) {
	return serviceapi.BearerAuth{Token: s.value}, nil
}
