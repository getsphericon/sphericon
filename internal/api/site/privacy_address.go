package site

import (
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/erasure"
)

// privacyAddress is the step the Privacy request operations (ADR 0021) share once Access has
// checked the Permission contacts:erase: a valid email address. A non-nil refusal is the 400 to
// answer an invalid address with; the operations have distinct generated response types, so each
// converts it itself.
func privacyAddress(email string) (*siteapi.ProblemDetails, error) {
	_, err := erasure.NormalizeEmail(email)
	if errors.Is(err, erasure.ErrInvalidEmail) {
		p := problem(http.StatusBadRequest, "invalid email address")
		return &p, nil
	}
	return nil, err
}
