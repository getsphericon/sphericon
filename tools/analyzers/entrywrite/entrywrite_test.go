package entrywrite_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/getsphericon/sphericon/tools/analyzers/entrywrite"
)

func TestEntrywrite(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), entrywrite.Analyzer,
		"github.com/getsphericon/sphericon/internal/jobs",
		"github.com/getsphericon/sphericon/internal/mcpserver",
		"github.com/getsphericon/sphericon/internal/api/site",
		"github.com/getsphericon/sphericon/internal/tags",
		"github.com/getsphericon/sphericon/internal/contacts",
		"github.com/getsphericon/sphericon/internal/usecase/tagdeletion",
		"github.com/getsphericon/sphericon/internal/testhelper",
		"github.com/getsphericon/sphericon/e2e",
		"github.com/getsphericon/sphericon/internal/automations",
		"github.com/getsphericon/sphericon/internal/drafts",
		"github.com/getsphericon/sphericon/ee/api/staff")
}
