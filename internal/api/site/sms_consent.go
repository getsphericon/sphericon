package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteSmsConsentsGet reads the SMS Confirmation and opt-out of the Contact's phone.
func (h *Handlers) SiteSmsConsentsGet(ctx context.Context, params siteapi.SiteSmsConsentsGetParams) (*siteapi.SmsConsent, error) {
	read, err := h.confirmations.ReadContactSMS(ctx, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}

// SiteSmsConsentsConfirm records the consent the Contact's phone gave outside sphericon.
func (h *Handlers) SiteSmsConsentsConfirm(ctx context.Context, req *siteapi.ConsentEvidence, params siteapi.SiteSmsConsentsConfirmParams) (*siteapi.SmsConsent, error) {
	read, err := h.confirmations.ConfirmContactSMS(ctx, workspaceScope(ctx), int64(params.ContactId), inputs.ConsentEvidence(req))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}

// SiteSmsConsentsOptOut records an SMS opt-out of the Contact's phone that reached the Workspace
// by another route.
func (h *Handlers) SiteSmsConsentsOptOut(ctx context.Context, params siteapi.SiteSmsConsentsOptOutParams) (*siteapi.SmsConsent, error) {
	read, err := h.channelStops.OptOutContactSMS(ctx, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	res := mapper.SmsConsentToResource(read)
	return &res, nil
}
