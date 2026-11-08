package driverops_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/driverops"
)

func TestDriverops(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), driverops.Analyzer,
		"github.com/getsphericon/sphericon/internal/api/site",
		"github.com/getsphericon/sphericon/internal/jobs",
		"github.com/getsphericon/sphericon/cmd/server",
		"github.com/getsphericon/sphericon/ee/api/staff",
		"github.com/getsphericon/sphericon/internal/usecase/compose")
}
