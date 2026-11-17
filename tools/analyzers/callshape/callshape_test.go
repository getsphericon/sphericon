package callshape_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/callshape"
	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

func TestUseCaseTier(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), callshape.Analyzer,
		"github.com/getsphericon/sphericon/internal/usecase/bad",
		"github.com/getsphericon/sphericon/internal/usecase/clean",
		"github.com/getsphericon/sphericon/internal/usecase/borrowed",
		"github.com/getsphericon/sphericon/ee/usecase/eebad")
}

func TestDomainTier(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), callshape.Analyzer,
		"github.com/getsphericon/sphericon/internal/domain/bad",
		"github.com/getsphericon/sphericon/internal/domain/clean")
}

// The analyzer never visits a package that does not exist, so it could not report such an entry as
// stale: an entry that names no package of the domain tier fails here instead.
func TestEveryExemptionNamesADomainPackage(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	seen := map[string]bool{}

	for _, path := range callshape.ExemptPackages() {
		assert.False(t, seen[path], "%s is listed twice", path)
		seen[path] = true
		assert.True(t, tier.IsDomain(tier.Module+path), "%s is not a package of the domain tier", path)
		assert.True(t, hasGoSource(t, filepath.Join(root, path)), "%s holds no Go package", path)
	}
}

func hasGoSource(t *testing.T, dir string) bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

func TestExemptPackageReportsNothingUntilItComplies(t *testing.T) {
	analyzer := callshape.New([]string{"internal/domain/exempt", "internal/domain/stale"})

	analysistest.Run(t, analysistest.TestData(), analyzer,
		"github.com/getsphericon/sphericon/internal/domain/exempt",
		"github.com/getsphericon/sphericon/internal/domain/stale")
}
