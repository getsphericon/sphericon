package entityupdate_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/entityupdate"
)

func TestEntityupdate(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), entityupdate.Analyzer,
		"github.com/getsphericon/sphericon/internal/bad",
		"github.com/getsphericon/sphericon/internal/clean",
		"github.com/getsphericon/sphericon/internal/testhelper")
}
