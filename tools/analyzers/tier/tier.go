// Package tier is the one statement of the backend's tiers that the analyzers read (ADR 0102,
// ADR 0111): which packages are the wiring, the drivers, the use cases and the domain. It names the
// roots of the components that easement.yml orders in `layers`, and a test checks the two agree, so a
// new driver or a moved tier changes both or fails.
//
// The drivers receive input from outside the program: an HTTP handler of a surface or of a public
// endpoint with the security handlers and the Problem mapping the surfaces share (which easement
// puts in a row below the other drivers so that they can import them), MCP, the job runners and the
// command line. The wiring builds the program and handles no input: the composition root, the
// router with its middleware (internal/server) and the Enterprise Edition (ee/ee.go), which easement
// lists beside the drivers only to fence what it imports.
package tier

import "strings"

// Module is the import path of the repository's Go module, with its trailing slash.
const Module = "github.com/getsphericon/sphericon/"

// Tier is where a package sits; None is a package of no tier named here (the shared surface code,
// the generated packages, the infrastructure, the kernel, a tool).
type Tier int

const (
	None Tier = iota
	Wiring
	Driver
	UseCase
	Domain
)

// handlers are the drivers that answer an HTTP request: the API surfaces, the OAuth server, the
// public endpoints with no contract (the tracked links, the Ingest hook, the edge's TLS question) and
// the Enterprise surfaces.
var handlers = []string{
	"internal/api/site",
	"internal/api/external",
	"internal/api/collect",
	"internal/api/staff",
	"internal/api/oauth",
	"internal/api/tracking",
	"internal/api/hooks",
	"internal/api/tlsask",
	"ee/api",
}

// drivers are every package that receives input from outside the program: the handlers with the
// security handlers and the Problem mapping they share, MCP, the job runners of the core and of the
// Enterprise Edition, and the command line with its driver package.
var drivers = append([]string{
	"internal/api/auth",
	"internal/api/problem",
	"internal/mcpserver",
	"internal/jobs",
	"ee/jobs",
	"cmd/server",
	"internal/commandline",
}, handlers...)

// wiring builds the program: the composition root, the router, the test harness, the doctor and the
// health check. The Edition is the package ee alone (edition), not the tree under it.
var wiring = []string{
	"internal/app",
	"internal/server",
	"internal/testhelper",
	"internal/doctor",
	"internal/healthcheck",
}

// edition is the Enterprise Edition's assembly, ee/ee.go.
const edition = "ee"

var useCases = []string{"internal/usecase", "ee/usecase"}

var domain = []string{"internal/domain", "ee/domain"}

// Of is the tier of the package at the import path.
func Of(path string) Tier {
	rel, ok := strings.CutPrefix(path, Module)
	if !ok {
		return None
	}
	switch {
	case rel == edition || under(rel, wiring):
		return Wiring
	case under(rel, drivers):
		return Driver
	case under(rel, useCases):
		return UseCase
	case under(rel, domain):
		return Domain
	}
	return None
}

// IsDriver holds for a package that receives input from outside the program.
func IsDriver(path string) bool { return Of(path) == Driver }

// IsHandler holds for a driver that answers an HTTP request.
func IsHandler(path string) bool {
	rel, ok := strings.CutPrefix(path, Module)
	return ok && under(rel, handlers)
}

// IsUseCase holds for a package of either use case tier.
func IsUseCase(path string) bool { return Of(path) == UseCase }

// IsDomain holds for a package of either domain tier.
func IsDomain(path string) bool { return Of(path) == Domain }

// IsCore holds for a package of the domain or the use case tier, of either edition: what a driver
// calls.
func IsCore(path string) bool {
	t := Of(path)
	return t == UseCase || t == Domain
}

// Roots are the directories, relative to the repository, that the statement names for the tier.
func Roots(t Tier) []string {
	switch t {
	case Wiring:
		return append([]string{edition}, wiring...)
	case Driver:
		return append([]string(nil), drivers...)
	case UseCase:
		return append([]string(nil), useCases...)
	case Domain:
		return append([]string(nil), domain...)
	case None:
	}
	return nil
}

// under holds for rel or a package below it in one of roots.
func under(rel string, roots []string) bool {
	for _, root := range roots {
		if rel == root || strings.HasPrefix(rel, root+"/") {
			return true
		}
	}
	return false
}
