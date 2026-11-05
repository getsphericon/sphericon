package testhelper

import (
	"testing"

	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
)

// statusProblem is the error a surface's client returns for a refused or failed call: the
// contract's Problem with the status it came with (each surface's *ProblemDetailsStatusCode).
type statusProblem[P any] interface {
	error
	GetStatusCode() int
	GetResponse() P
}

// refusal is the problem err carries, checked to have come with status. msgAndArgs, as testify
// takes them, say what the refusal is.
func refusal[P any, E statusProblem[P]](tb testing.TB, err error, status int, msgAndArgs ...any) P {
	tb.Helper()
	var refused E
	require.ErrorAs(tb, err, &refused, msgAndArgs...)
	require.Equal(tb, status, refused.GetStatusCode(), msgAndArgs...)
	return refused.GetResponse()
}

// SiteRefusal is the problem a /site call was refused with, checked to have come with status.
func SiteRefusal(tb testing.TB, err error, status int, msgAndArgs ...any) siteapi.ProblemDetails {
	tb.Helper()
	return refusal[siteapi.ProblemDetails, *siteapi.ProblemDetailsStatusCode](tb, err, status, msgAndArgs...)
}

// ExternalRefusal is the problem an /api call was refused with, checked to have come with status.
func ExternalRefusal(tb testing.TB, err error, status int, msgAndArgs ...any) externalapi.ProblemDetails {
	tb.Helper()
	return refusal[externalapi.ProblemDetails, *externalapi.ProblemDetailsStatusCode](tb, err, status, msgAndArgs...)
}

// CollectRefusal is the problem a /collect call was refused with, checked to have come with status.
func CollectRefusal(tb testing.TB, err error, status int, msgAndArgs ...any) collectapi.ProblemDetails {
	tb.Helper()
	return refusal[collectapi.ProblemDetails, *collectapi.ProblemDetailsStatusCode](tb, err, status, msgAndArgs...)
}

// StaffRefusal is the problem a core Staff call was refused with, checked to have come with status.
func StaffRefusal(tb testing.TB, err error, status int, msgAndArgs ...any) staffapi.ProblemDetails {
	tb.Helper()
	return refusal[staffapi.ProblemDetails, *staffapi.ProblemDetailsStatusCode](tb, err, status, msgAndArgs...)
}

// StaffeeRefusal is the problem a call of the Staff console's Enterprise sections was refused
// with, checked to have come with status.
func StaffeeRefusal(tb testing.TB, err error, status int, msgAndArgs ...any) staffeeapi.ProblemDetails {
	tb.Helper()
	return refusal[staffeeapi.ProblemDetails, *staffeeapi.ProblemDetailsStatusCode](tb, err, status, msgAndArgs...)
}

// ServiceRefusal is the problem a service endpoint call was refused with, checked to have come
// with status.
func ServiceRefusal(tb testing.TB, err error, status int, msgAndArgs ...any) serviceapi.ProblemDetails {
	tb.Helper()
	return refusal[serviceapi.ProblemDetails, *serviceapi.ProblemDetailsStatusCode](tb, err, status, msgAndArgs...)
}
