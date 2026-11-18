package site_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/kernel/deferralcause"
	"github.com/getsphericon/sphericon/internal/kernel/skipreason"
)

// The site's contract lists the Skipped reasons and the Deferral causes as enums, each exactly the
// kernel's closed list, so a client can switch over them (ADR 0178).
func TestTheSiteContractListsTheSkippedReasonsAndTheDeferralCauses(t *testing.T) {
	enums := contractEnums(t, "../../../openapi/site.openapi.json")

	assert.ElementsMatch(t, skipreason.Reason("").Values(), enums["SkippedReason"])
	assert.ElementsMatch(t, deferralcause.Cause("").Values(), enums["DeferralCause"])
}

// contractEnums reads the enum values of every schema of the OpenAPI document at path.
func contractEnums(t *testing.T, path string) map[string][]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Enum []string `json:"enum"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	enums := map[string][]string{}
	for name, s := range doc.Components.Schemas {
		enums[name] = s.Enum
	}
	return enums
}
