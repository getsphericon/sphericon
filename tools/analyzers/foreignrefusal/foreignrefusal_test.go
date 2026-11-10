package foreignrefusal_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/foreignrefusal"
)

func TestForeignrefusal(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), foreignrefusal.Analyzer, "owner", "owner/ownerunchecked", "bad", "clean")
}
