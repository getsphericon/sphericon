package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/consent"
)

// SiteSmsConsentsGet reads the SMS Confirmation and opt-out of the Contact's phone.
func (h *Handlers) SiteSmsConsentsGet(ctx context.Context, params siteapi.SiteSmsConsentsGetParams) (siteapi.SiteSmsConsentsGetRes, error) {
	read, err := consent.ReadContactSMS(ctx, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}

// SiteSmsConsentsConfirm records the consent the Contact's phone gave outside sphericon.
func (h *Handlers) SiteSmsConsentsConfirm(ctx context.Context, req *siteapi.ConsentEvidence, params siteapi.SiteSmsConsentsConfirmParams) (siteapi.SiteSmsConsentsConfirmRes, error) {
	read, err := consent.ConfirmContactSMS(ctx, h.bus, workspaceScope(ctx), int64(params.ContactId), inputs.ConsentEvidence(req))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}

// SiteSmsConsentsOptOut records an SMS opt-out of the Contact's phone that reached the Workspace
// by another route.
func (h *Handlers) SiteSmsConsentsOptOut(ctx context.Context, params siteapi.SiteSmsConsentsOptOutParams) (siteapi.SiteSmsConsentsOptOutRes, error) {
	read, err := consent.OptOutContactSMS(ctx, h.bus, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}
