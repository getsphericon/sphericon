package credentials_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/credentials"
)

// A presented API token or Service credential comes from an Authorization header: a value parses
// as at most one of the two kinds, and only when it is exactly the value its parts make.
func FuzzPresentedCredentialParsesAsItsOwnKindOnly(f *testing.F) {
	prefix, secret := strings.Repeat("p", 12), strings.Repeat("s", 40)
	f.Add(credentials.TokenValue(prefix, secret))
	f.Add(credentials.ServiceCredentialValue(prefix, secret))
	f.Add("omtk_" + prefix + "_" + secret + "\n")
	f.Add("Bearer omtk_" + prefix + "_" + secret)
	f.Add("smsc_omtk_" + prefix + "_" + secret)
	f.Add("")

	f.Fuzz(func(t *testing.T, value string) {
		token := credentials.ParseToken(value)
		service := credentials.ParseServiceCredential(value)

		assert.Truef(t, token == nil || service == nil, "%q parses as both kinds", value)
		assert.Truef(t, token == nil || credentials.TokenValue(token.Prefix, token.Secret) == value, "%q parses as token %+v", value, token)
		assert.Truef(t, service == nil || credentials.ServiceCredentialValue(service.Prefix, service.Secret) == value, "%q parses as credential %+v", value, service)
	})
}
