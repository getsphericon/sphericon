package noprint_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/noprint"
)

func TestNoprint(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), noprint.Analyzer, "bad", "clean")
}
