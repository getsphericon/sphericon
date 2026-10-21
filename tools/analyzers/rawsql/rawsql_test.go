package rawsql_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/rawsql"
)

func TestRawsql(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), rawsql.Analyzer, "bad", "clean")
}
