package scopegen

import (
	"encoding/json"
	"fmt"
	"strings"
)

// rateLimitExtension is the policy under which the server limits an operation per client address
// (`@rateLimited`): login, forgot_password or human.
const rateLimitExtension = "x-rate-limit"

// SiteRateLimits is the /site part of the rate limiter's routes: each operation the contract marks
// `@rateLimited`, as the ServeMux pattern of its method and full path, with its policy.
var SiteRateLimits = rateLimits(siteSpec, "site", "Site", "siteLimits")

// StaffRateLimits is the /staff part of the rate limiter's routes, as SiteRateLimits is /site's.
var StaffRateLimits = rateLimits("openapi/staff.openapi.json", "staff", "Staff", "staffLimits")

// rateLimits is the table of a surface's limited operations, written into the rate limiter as the
// map name: the surface's paths are under paths.<prefix>.
func rateLimits(spec, surface, prefix, name string) Table {
	return Table{
		Spec:    spec,
		Out:     "internal/infra/ratelimit/" + surface + "_limits_gen.go",
		Package: "ratelimit",
		Imports: []string{`"github.com/getsphericon/sphericon/internal/kernel/paths"`},
		Doc: []string{
			name + " are the operations of /" + surface + " the contract marks `@rateLimited`: the ServeMux",
			"pattern of each, under its surface's prefix, and the policy it is limited under.",
		},
		Var:  name,
		Type: "map[string]RouteLimit",
		Rows: func(doc Document) ([]string, error) {
			var rows []string
			for _, op := range doc.Operations {
				raw, ok := op.Extensions[rateLimitExtension]
				if !ok {
					continue
				}
				var policy string
				if err := json.Unmarshal(raw, &policy); err != nil || policy == "" {
					return nil, fmt.Errorf("%s: %s is not a policy name", op, rateLimitExtension)
				}
				pattern := fmt.Sprintf("%q + paths.%s + %q", strings.ToUpper(op.Method)+" ", prefix, op.Path)
				rows = append(rows, fmt.Sprintf("%s: RouteLimit%s", pattern, valueName(policy)))
			}
			return rows, nil
		},
	}
}
