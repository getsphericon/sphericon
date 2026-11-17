package callshape_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/callshape"
)

func TestUseCaseTier(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), callshape.Analyzer,
		"github.com/getsphericon/sphericon/internal/usecase/bad",
		"github.com/getsphericon/sphericon/internal/usecase/clean",
		"github.com/getsphericon/sphericon/internal/usecase/borrowed",
		"github.com/getsphericon/sphericon/ee/usecase/eebad")
}

func TestDomainTier(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), callshape.Analyzer,
		"github.com/getsphericon/sphericon/internal/domain/bad",
		"github.com/getsphericon/sphericon/internal/domain/clean")
}
