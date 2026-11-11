package contractrevision_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/contractrevision"
)

func check(t *testing.T, path string) contractrevision.Report {
	t.Helper()
	doc, err := os.ReadFile(path)
	require.NoError(t, err)
	r, err := contractrevision.Check(doc)
	require.NoError(t, err)
	return r
}

// Every update of a resource with a revision, on every surface, requires it in its body.
func TestEveryUpdateOfAResourceWithARevisionRequiresIt(t *testing.T) {
	docs, err := filepath.Glob("../../openapi/*.openapi.json")
	require.NoError(t, err)
	require.NotEmpty(t, docs)
	for _, path := range docs {
		assert.Empty(t, check(t, path).Missing, path)
	}
}

// The check sees the Template update on both surfaces that have one, so it cannot pass by reading
// nothing (a resource schema it could not resolve).
func TestTheCheckExaminesTheTemplateUpdateOfEachSurface(t *testing.T) {
	assert.Contains(t, check(t, "../../openapi/site.openapi.json").Examined, "PUT /workspaces/{workspaceId}/templates/{id}")
	assert.Contains(t, check(t, "../../openapi/external.openapi.json").Examined, "PUT /templates/{id}")
}

// The check names an update whose body leaves the revision optional, and only that one.
func TestTheCheckNamesAnUpdateWhoseBodyLeavesTheRevisionOut(t *testing.T) {
	r := check(t, "testdata/missing.openapi.json")

	assert.Equal(t, []string{"PUT /kept/{id}", "PUT /things/{id}"}, r.Examined)
	assert.Equal(t, []string{"PUT /things/{id}"}, r.Missing)
}
