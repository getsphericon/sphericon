package nowriter_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/nowriter"
)

func TestNowriter(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nowriter.Analyzer,
		"github.com/getsphericon/sphericon/internal/domain/bad",
		"github.com/getsphericon/sphericon/internal/domain/clean",
		"github.com/getsphericon/sphericon/ee/domain/eebad",
		"github.com/getsphericon/sphericon/internal/usecase/report",
		"github.com/getsphericon/sphericon/ee/usecase/eereport",
		"github.com/getsphericon/sphericon/internal/commandline",
		"github.com/getsphericon/sphericon/internal/server")
}
