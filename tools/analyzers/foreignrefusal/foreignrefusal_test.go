package foreignrefusal_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/foreignrefusal"
)

// domain is where the testdata puts its modules, under the module path as the real ones are.
const domain = "github.com/getsphericon/sphericon/internal/domain/"

func TestForeignrefusal(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), foreignrefusal.Analyzer,
		domain+"owner", domain+"owner/ownerunchecked", domain+"bad", domain+"clean")
}
