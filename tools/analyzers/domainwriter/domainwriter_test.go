package domainwriter_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/domainwriter"
)

func TestDomainwriter(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), domainwriter.Analyzer,
		"github.com/getsphericon/sphericon/internal/domain/bad",
		"github.com/getsphericon/sphericon/internal/domain/clean",
		"github.com/getsphericon/sphericon/ee/domain/eebad",
		"github.com/getsphericon/sphericon/internal/commandline")
}
