package tier_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/getsphericon/sphericon/tools/analyzers/tier"
)

// root is the repository the analyzers check, from this package's directory.
const root = "../../.."

func TestOfPlacesEachPackageInItsTier(t *testing.T) {
	cases := map[string]tier.Tier{
		"internal/app":                    tier.Wiring,
		"internal/server":                 tier.Wiring,
		"ee":                              tier.Wiring,
		"internal/api/site":               tier.Driver,
		"internal/api/site/resources":     tier.Driver,
		"ee/api/staff":                    tier.Driver,
		"internal/jobs":                   tier.Driver,
		"ee/jobs":                         tier.Driver,
		"internal/mcpserver":              tier.Driver,
		"cmd/server":                      tier.Driver,
		"internal/commandline":            tier.Driver,
		"internal/usecase/tagdeletion":    tier.UseCase,
		"ee/usecase/auditlogretention":    tier.UseCase,
		"internal/domain/contacts/tags":   tier.Domain,
		"ee/domain/insights/auditentries": tier.Domain,
		"internal/api/auth":               tier.Driver,
		"internal/api/problem/mapping":    tier.Driver,
		"internal/infra/events":           tier.None,
		"internal/kernel/batch":           tier.None,
		"internal/serverless":             tier.None,
		"ee/licensekey":                   tier.None,
		"cmd/seed":                        tier.None,
	}
	for rel, want := range cases {
		assert.Equal(t, want, tier.Of(tier.Module+rel), rel)
	}
	assert.Equal(t, tier.None, tier.Of("github.com/other/module/internal/app"))
}

func TestOnlyHTTPDriversAreHandlers(t *testing.T) {
	assert.True(t, tier.IsHandler(tier.Module+"internal/api/tracking"))
	assert.True(t, tier.IsHandler(tier.Module+"ee/api/staff"))
	assert.False(t, tier.IsHandler(tier.Module+"internal/api/auth"))
	assert.False(t, tier.IsHandler(tier.Module+"internal/jobs"))
	assert.False(t, tier.IsHandler(tier.Module+"cmd/server"))
}

// TestTheTiersAgreeWithEasementLayers reads easement.yml: each root the statement names is a
// component of `layers`; a row of `layers` that holds a driver holds only drivers and the wiring (the
// Edition sits beside the drivers for its fence), one that holds a use case or a domain module only
// those; and going down `layers` the tiers keep their order, wiring, drivers, use cases, domain.
func TestTheTiersAgreeWithEasementLayers(t *testing.T) {
	cfg := readEasement(t)

	assert.Empty(t, undeclared(cfg), "layers names components that components does not declare")
	assert.Empty(t, rowDisagreements(cfg), "rows of layers that disagree with the tiers")
	assert.Empty(t, missingRoots(cfg), "roots the tiers name that no component of layers holds")
}

// rowTiers is the tier of each root of each component in a row of layers.
func rowTiers(cfg easement, row names) []tier.Tier {
	var tiers []tier.Tier
	for _, name := range row {
		for _, in := range cfg.Components[name].In {
			tiers = append(tiers, tier.Of(tier.Module+globRoot(in)))
		}
	}
	return tiers
}

func undeclared(cfg easement) []string {
	var missing []string
	for _, row := range cfg.Layers {
		for _, name := range row {
			if _, ok := cfg.Components[name]; !ok {
				missing = append(missing, name)
			}
		}
	}
	return missing
}

// rowDisagreements describes each row of layers that mixes tiers, or sits above a tier it belongs
// below. A row of no tier the statement names (the shared code of the surfaces' generated packages,
// the infrastructure, the kernel) is skipped.
func rowDisagreements(cfg easement) []string {
	var bad []string
	rank := tier.None
	for i, row := range cfg.Layers {
		tiers := rowTiers(cfg, row)
		known := slices.DeleteFunc(slices.Clone(tiers), func(t tier.Tier) bool { return t == tier.None })
		if len(known) == 0 {
			continue
		}
		distinct := slices.Compact(slices.Sorted(slices.Values(tiers)))
		allowed := []tier.Tier{known[0]}
		if slices.Contains(known, tier.Driver) {
			allowed = []tier.Tier{tier.Driver, tier.Wiring}
		}
		if slices.ContainsFunc(distinct, func(t tier.Tier) bool { return !slices.Contains(allowed, t) }) {
			bad = append(bad, fmt.Sprintf("row %d (%v) mixes tiers %v", i, row, distinct))
		}
		if slices.Min(known) < rank {
			bad = append(bad, fmt.Sprintf("row %d (%v) sits above a tier it belongs below", i, row))
		}
		rank = max(rank, slices.Max(known))
	}
	return bad
}

func missingRoots(cfg easement) []string {
	var roots []string
	for _, row := range cfg.Layers {
		for _, name := range row {
			for _, in := range cfg.Components[name].In {
				roots = append(roots, globRoot(in))
			}
		}
	}
	var missing []string
	for _, tr := range []tier.Tier{tier.Wiring, tier.Driver, tier.UseCase, tier.Domain} {
		for _, r := range tier.Roots(tr) {
			if !slices.ContainsFunc(roots, func(c string) bool { return c == r || strings.HasPrefix(c, r+"/") }) {
				missing = append(missing, r)
			}
		}
	}
	return missing
}

type easement struct {
	Components map[string]struct {
		In names `yaml:"in"`
	} `yaml:"components"`
	Layers []names `yaml:"layers"`
}

// names is a YAML value that is one string or a list of them.
type names []string

func (n *names) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		*n = names{value.Value}
		return nil
	}
	var list []string
	if err := value.Decode(&list); err != nil {
		return err
	}
	*n = list
	return nil
}

func readEasement(t *testing.T) easement {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "easement.yml"))
	require.NoError(t, err)
	var cfg easement
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotEmpty(t, cfg.Layers)
	return cfg
}

// globRoot is the directory a component's pattern starts from: its segments before the first one
// that holds a wildcard.
func globRoot(pattern string) string {
	var segs []string
	for seg := range strings.SplitSeq(pattern, "/") {
		if strings.ContainsAny(seg, "*?[{") {
			break
		}
		segs = append(segs, seg)
	}
	return strings.Join(segs, "/")
}
