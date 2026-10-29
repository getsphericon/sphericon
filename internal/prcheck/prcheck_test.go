package prcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/prcheck"
)

func TestCheckDisclosureAcceptsOneCheckedOption(t *testing.T) {
	for name, body := range map[string]string{
		"first":         "## Summary\n\nx\n\n## AI assistance\n\n- [x] No AI tool was used.\n- [ ] An AI tool helped.\n- [ ] An AI tool wrote it.\n",
		"last":          "## AI assistance\n\n- [ ] No AI tool was used.\n- [ ] An AI tool helped.\n- [X] An AI tool wrote it.\n",
		"before others": "## AI assistance\n\n- [ ] No AI tool was used.\n- [x] An AI tool helped.\n\n## Checklist\n\n- [x] Docs\n",
		"star bullets":  "## AI assistance\n\n* [x] No AI tool was used.\n* [ ] An AI tool helped.\n",
		"windows lines": "## AI assistance\r\n\r\n- [x] No AI tool was used.\r\n- [ ] An AI tool helped.\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, prcheck.CheckDisclosure(body))
		})
	}
}

func TestCheckDisclosureRefusesAnythingElse(t *testing.T) {
	for name, tc := range map[string]struct{ body, want string }{
		"no section":                 {"## Summary\n\nx\n", "no \"## AI assistance\" section"},
		"empty body":                 {"", "no \"## AI assistance\" section"},
		"none checked":               {"## AI assistance\n\n- [ ] No AI tool was used.\n- [ ] An AI tool helped.\n", "found 0 checked"},
		"two checked":                {"## AI assistance\n\n- [x] No AI tool was used.\n- [x] An AI tool helped.\n", "found 2 checked"},
		"a checked box elsewhere":    {"## AI assistance\n\n- [ ] No AI tool was used.\n\n## Checklist\n\n- [x] Docs\n", "found 0 checked"},
		"section named by prose":     {"The AI assistance section is below.\n- [x] fine\n", "no \"## AI assistance\" section"},
		"heading of another level":   {"### AI assistance\n\n- [x] No AI tool was used.\n", "no \"## AI assistance\" section"},
		"box without a label":        {"## AI assistance\n\n- [x]\n- [ ] An AI tool helped.\n", "found 0 checked"},
		"heading with trailing text": {"## AI assistance (required)\n\n- [x] No AI tool was used.\n", "no \"## AI assistance\" section"},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, prcheck.CheckDisclosure(tc.body), tc.want)
		})
	}
}

func TestAddedMarkerFiles(t *testing.T) {
	diff := "diff --git a/internal/x/limits.go b/internal/x/limits.go\n" +
		"--- a/internal/x/limits.go\n+++ b/internal/x/limits.go\n@@ -1,0 +2,2 @@\n" +
		"+// contract-copy: a secret floor\n+const Min = 8\n" +
		"diff --git a/src/limits.ts b/src/limits.ts\n--- a/src/limits.ts\n+++ b/src/limits.ts\n@@ -1,0 +1 @@\n" +
		"+// contract-copy: an OAuth limit\n" +
		"diff --git a/fixtures/a.yml b/fixtures/a.yml\n--- a/fixtures/a.yml\n+++ b/fixtures/a.yml\n@@ -1,0 +1 @@\n" +
		"+  # contract-copy: scope the contract has no name for\n" +
		"diff --git a/docs/x.md b/docs/x.md\n--- a/docs/x.md\n+++ b/docs/x.md\n@@ -1,0 +1 @@\n" +
		"+Mark a value `// contract-copy: <reason>`.\n" +
		"diff --git a/internal/contractcopy/scan.go b/internal/contractcopy/scan.go\n--- a/internal/contractcopy/scan.go\n" +
		"+++ b/internal/contractcopy/scan.go\n@@ -1,0 +1 @@\n+// contract-copy: text about a marker\n" +
		"diff --git a/internal/y/y.go b/internal/y/y.go\n--- a/internal/y/y.go\n+++ b/internal/y/y.go\n@@ -3 +3 @@\n" +
		"-// contract-copy: removed, not added\n+// a plain comment\n" +
		"+// contract-copy: second marker in the file\n+// contract-copy:\n"

	assert.Equal(t, []string{"fixtures/a.yml", "internal/x/limits.go", "internal/y/y.go", "src/limits.ts"}, prcheck.AddedMarkerFiles(diff))
}

func TestCheckMarkersAcceptsDeclaredFilesAndNoMarkers(t *testing.T) {
	for name, tc := range map[string]struct {
		files []string
		body  string
	}{
		"no marker":         {nil, ""},
		"declared":          {[]string{"internal/x/limits.go"}, "Notes\n\nContract-Copy: internal/x/limits.go: a secret floor\n"},
		"two files":         {[]string{"a.go", "b.go"}, "Contract-Copy: a.go: why\nContract-Copy: b.go: because\n"},
		"declared with tab": {[]string{"a.go"}, "Contract-Copy:\ta.go:\twhy\n"},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, prcheck.CheckMarkers(tc.files, tc.body))
		})
	}
}

func TestCheckMarkersNamesWhatTheBodyLeavesOut(t *testing.T) {
	for name, tc := range map[string]struct {
		files []string
		body  string
		want  string
	}{
		"no declaration":      {[]string{"a.go"}, "", "Contract-Copy: a.go:"},
		"another file":        {[]string{"a.go"}, "Contract-Copy: b.go: why\n", "Contract-Copy: a.go:"},
		"no reason":           {[]string{"a.go"}, "Contract-Copy: a.go:\n", "Contract-Copy: a.go:"},
		"not at a line start": {[]string{"a.go"}, "see Contract-Copy: a.go: why\n", "Contract-Copy: a.go:"},
		"one of two":          {[]string{"a.go", "b.go"}, "Contract-Copy: a.go: why\n", "Contract-Copy: b.go:"},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, prcheck.CheckMarkers(tc.files, tc.body), tc.want)
		})
	}
}
