package handmapper_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/handmapper"
)

func TestHandmapper(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), handmapper.Analyzer, "bad", "clean")
}
