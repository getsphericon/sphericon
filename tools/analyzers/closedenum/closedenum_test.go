package closedenum_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/closedenum"
)

// internal is where the testdata puts its packages, under the module path as the real ones are.
const internal = "github.com/getsphericon/sphericon/internal/"

func TestClosedenum(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), closedenum.Analyzer, internal+"medium", internal+"bad", internal+"clean")
}
