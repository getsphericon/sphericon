package testwrites_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/testwrites"
)

func TestTestwrites(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), testwrites.Analyzer, "bad", "clean")
}
