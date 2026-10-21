// Package analyzers is the golangci-lint module plugin that carries the project's own
// go/analysis checks (docs/agents/review-lenses.md). `golangci-lint custom` builds it into
// .bin/custom-gcl through .custom-gcl.yml; .golangci.yml enables it as the linter "sphericon".
package analyzers

import (
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/getsphericon/sphericon/tools/analyzers/noprint"
)

func init() {
	register.Plugin("sphericon", New)
}

// Plugin is the linter "sphericon": every analyzer of this module.
type Plugin struct{}

// New builds the plugin; it takes no settings.
func New(any) (register.LinterPlugin, error) {
	return Plugin{}, nil
}

// BuildAnalyzers lists the analyzers; add a new one here.
func (Plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{noprint.Analyzer}, nil
}

// GetLoadMode asks for types, which every analyzer here needs.
func (Plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
