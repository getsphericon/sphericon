package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/consent"
)

// SmsConsentsGet reads the SMS Confirmation and opt-out of the Contact's phone, as the Contact card
// shows them.
func (h *Handlers) SmsConsentsGet(ctx context.Context, params externalapi.SmsConsentsGetParams) (externalapi.SmsConsentsGetRes, error) {
	read, err := consent.ReadContactSMS(ctx, auth.TokenScoped(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}
