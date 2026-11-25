package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/sending/confirmationrequests"
)

// ConfirmationRequestsCreate asks for a double opt-in confirmation email to a Contact (ADR 0013):
// 202 while it is on its way, 200 when the address is confirmed already.
func (h *Handlers) ConfirmationRequestsCreate(ctx context.Context, params externalapi.ConfirmationRequestsCreateParams) (externalapi.ConfirmationRequestsCreateRes, error) {
	status, err := h.confirmationRequests.AskByWorkspace(ctx, auth.TokenScoped(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	switch status {
	case confirmationrequests.Confirmed:
		return &externalapi.ConfirmationRequestsCreateOK{Status: externalapi.ConfirmationRequestStatusConfirmed}, nil
	case confirmationrequests.Accepted:
	}
	return &externalapi.ConfirmationRequestsCreateAccepted{Status: externalapi.ConfirmationRequestStatusAccepted}, nil
}
