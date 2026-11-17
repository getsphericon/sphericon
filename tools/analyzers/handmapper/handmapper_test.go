package handmapper_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/handmapper"
)

func TestHandmapper(t *testing.T) {
	const m = "github.com/getsphericon/sphericon/"
	analysistest.Run(t, analysistest.TestData(), handmapper.Analyzer, "bad", "clean",
		m+"internal/api/site", m+"internal/api/site/resources", m+"internal/api/external",
		m+"internal/api/staff", m+"ee/api/staff", m+"internal/domain/automations", m+"internal/server")
}
