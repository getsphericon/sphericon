package driverops_test

import (
	"go/types"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/driverops"
)

func TestDriverops(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), driverops.Analyzer,
		"github.com/getsphericon/sphericon/internal/api/site",
		"github.com/getsphericon/sphericon/internal/jobs",
		"github.com/getsphericon/sphericon/cmd/server",
		"github.com/getsphericon/sphericon/ee/api/staff",
		"github.com/getsphericon/sphericon/internal/usecase/compose",
		"github.com/getsphericon/sphericon/internal/commandline",
		"github.com/getsphericon/sphericon/internal/server",
		"github.com/getsphericon/sphericon/internal/api/auth")
}

// TestAMissingControlFlowGraphFailsTheRun: a driver package whose control-flow graphs the runner
// did not provide is an error, not a package that passes unread.
func TestAMissingControlFlowGraphFailsTheRun(t *testing.T) {
	pass := &analysis.Pass{
		Pkg:      types.NewPackage("github.com/getsphericon/sphericon/internal/api/site", "site"),
		ResultOf: map[*analysis.Analyzer]any{},
	}

	_, err := driverops.Analyzer.Run(pass)

	require.ErrorContains(t, err, "no result of ctrlflow for github.com/getsphericon/sphericon/internal/api/site")
}
