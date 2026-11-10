package prcheck_test

import (
	"testing"

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
