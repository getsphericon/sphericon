package statemoment_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/statemoment"
)

func TestStatemoment(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), statemoment.Analyzer,
		"github.com/getsphericon/sphericon/internal/bad",
		"github.com/getsphericon/sphericon/internal/clean")
}
