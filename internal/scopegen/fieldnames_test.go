package scopegen_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/scopegen"
)

func TestGenerateFieldNamesNamesEveryRequestFieldOfEveryDocument(t *testing.T) {
	site := []byte(`{"paths":{"/integrations/{id}":{"patch":{"operationId":"SiteIntegrations_update",
	  "parameters":[{"name":"id","in":"path"}],
	  "requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/UpdateIntegrationInput"}}}}}}},
	 "components":{"schemas":{
	  "UpdateIntegrationInput":{"type":"object","properties":{"perDay":{"type":"integer"},"limits":{"$ref":"#/components/schemas/Limits"}}},
	  "Limits":{"type":"object","properties":{"maxPerSecond":{"type":"integer"}}},
	  "IntegrationResource":{"type":"object","properties":{"createdAt":{"type":"string"}}}}}}`)
	external := []byte(`{"paths":{"/contacts":{"post":{"operationId":"Contacts_create",
	  "requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"perDay":{"type":"integer"},"redirect_uris":{"type":"array"}}}}}}}}}}`)

	src, err := scopegen.FieldNames.Generate(map[string][]byte{"site": site, "external": external})
	require.NoError(t, err)

	out := string(src)
	require.Contains(t, out, "package fieldname")
	require.Regexp(t, `PerDay\s+= "perDay"`, out)
	require.Regexp(t, `MaxPerSecond\s+= "maxPerSecond"`, out)
	require.Regexp(t, `ID\s+= "id"`, out)
	require.Regexp(t, `RedirectURIs\s+= "redirect_uris"`, out)
	require.NotContains(t, out, "createdAt", "a response's field is no request field")
}

func TestGenerateFieldNamesRefusesTwoFieldsOfOneGoName(t *testing.T) {
	doc := []byte(`{"paths":{"/x":{"post":{"operationId":"X_create",
	  "requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"redirectUris":{},"redirect_uris":{}}}}}}}}}}`)

	_, err := scopegen.FieldNames.Generate(map[string][]byte{"site": doc})

	require.ErrorContains(t, err, "RedirectURIs")
}
