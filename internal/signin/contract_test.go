package signin_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// contractSigninErrors reads the SiteSigninError enum from the generated /site contract:
// ogen emits no Go type for an enum no response references, so the Reason constants are
// kept equal to it by test.
func contractSigninErrors(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../openapi/site.openapi.json")
	require.NoError(t, err)
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `json:"enum"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	enum := doc.Components.Schemas["SiteSigninError"].Enum
	require.NotEmpty(t, enum, "the contract declares no SiteSigninError")
	return enum
}
