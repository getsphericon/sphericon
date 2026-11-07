package usecaseshape_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/usecaseshape"
)

func TestUsecaseshape(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), usecaseshape.Analyzer,
		"github.com/getsphericon/sphericon/internal/usecase/bad",
		"github.com/getsphericon/sphericon/internal/usecase/clean",
		"github.com/getsphericon/sphericon/internal/domain/other",
		"github.com/getsphericon/sphericon/ee/usecase/eebad")
}
