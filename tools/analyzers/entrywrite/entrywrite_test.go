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
		"github.com/getsphericon/sphericon/internal/domain/contacts/tags",
		"github.com/getsphericon/sphericon/internal/domain/contacts/contacts",
		"github.com/getsphericon/sphericon/internal/usecase/tagdeletion",
		"github.com/getsphericon/sphericon/internal/usecase/workspacedeletion",
		"github.com/getsphericon/sphericon/internal/testhelper",
		"github.com/getsphericon/sphericon/e2e",
		"github.com/getsphericon/sphericon/internal/domain/orchestration/automations",
		"github.com/getsphericon/sphericon/internal/domain/content/versioneddefinition",
		"github.com/getsphericon/sphericon/ee/api/staff",
		"github.com/getsphericon/sphericon/ee/jobs")
}
