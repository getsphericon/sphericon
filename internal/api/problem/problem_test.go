package problem_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/problem"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

var errMissing = validation.NotFound("thing not found", i18n.ErrorsContactNotFound)

var errForbidden = validation.Forbidden("not allowed", i18n.ErrorsContactNotFound)

// answer is what a surface's NewError answered: the status, the code and the detail.
type answer struct {
	status       int
	code, detail string
}

// surfaces are the NewError of every surface, each read as an answer.
var surfaces = map[string]func(context.Context, error) answer{
	"site": func(ctx context.Context, err error) answer {
		p := problem.Site(ctx, err)
		return answer{p.StatusCode, string(p.Response.Code.Or("")), p.Response.Detail.Or("")}
	},
	"external": func(ctx context.Context, err error) answer {
		p := problem.External(ctx, err)
		return answer{p.StatusCode, string(p.Response.Code.Or("")), p.Response.Detail.Or("")}
	},
	"collect": func(ctx context.Context, err error) answer {
		p := problem.Collect(ctx, err)
		return answer{p.StatusCode, string(p.Response.Code.Or("")), p.Response.Detail.Or("")}
	},
	"staff": func(ctx context.Context, err error) answer {
		p := problem.Staff(ctx, err)
		return answer{p.StatusCode, string(p.Response.Code.Or("")), p.Response.Detail.Or("")}
	},
	"staffee": func(ctx context.Context, err error) answer {
		p := problem.Staffee(ctx, err)
		return answer{p.StatusCode, string(p.Response.Code.Or("")), p.Response.Detail.Or("")}
	},
	"service": func(ctx context.Context, err error) answer {
		p := problem.Service(ctx, err)
		return answer{p.StatusCode, string(p.Response.Code.Or("")), p.Response.Detail.Or("")}
	},
}

// Every surface answers a refusal with its own copy of the one problem: the status, the detail and
// the code, which stays unset where the refusal carries none. A refusal's detail is its message in the
// catalog.
func TestEverySurfaceAnswersARefusalAlike(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want answer
	}{
		"a refusal without a code": {errMissing, answer{http.StatusNotFound, "", "contact not found"}},
		"a refusal with a code":    {errForbidden, answer{http.StatusForbidden, "permission_required", "contact not found"}},
		"an answer of a handler":   {&problem.Error{Status: http.StatusPaymentRequired, Detail: "licensed"}, answer{http.StatusPaymentRequired, "", "licensed"}},
		"a fault":                  {errors.New("boom"), answer{http.StatusInternalServerError, "", ""}},
	} {
		t.Run(name, func(t *testing.T) {
			for surface, of := range surfaces {
				assert.Equal(t, tc.want, of(context.Background(), tc.err), surface)
			}
		})
	}
}

// A refusal's code reaches the external surface as its own enum member.
func TestARefusalCodeIsTheSurfacesOwnMember(t *testing.T) {
	got := problem.External(context.Background(), errForbidden)
	assert.Equal(t, externalapi.NewOptProblemCode(externalapi.ProblemCodePermissionRequired), got.Response.Code)
}
