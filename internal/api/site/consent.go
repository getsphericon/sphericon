package site

import (
	"context"
	"fmt"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/clientip"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// SitePublicConfirmationsPerform is the double opt-in confirmation page's button
// (ADR 0013): the one place a confirmation is performed.
func (h *Handlers) SitePublicConfirmationsPerform(ctx context.Context, params siteapi.SitePublicConfirmationsPerformParams) (siteapi.SitePublicConfirmationsPerformRes, error) {
	target, err := h.tracker.DecodeConfirm(params.Token)
	if tracking.IsExpired(err) {
		v := siteapi.SitePublicConfirmationsPerformGone(problem(http.StatusGone, "confirmation link expired"))
		return &v, nil
	}
	if err != nil {
		v := siteapi.SitePublicConfirmationsPerformBadRequest(problem(http.StatusBadRequest, "invalid token"))
		return &v, nil
	}
	if err := consent.RecordConfirmation(ctx, h.bus, target, clientip.FromContext(ctx)); err != nil {
		return nil, fmt.Errorf("record confirmation: %w", err)
	}
	return &siteapi.SitePublicConfirmationsPerformNoContent{}, nil
}

// SitePublicUnsubscribesPerform is the unsubscribe page's button (ADR 0012): the
// same effect as POST /e/u/{token}, which stays for the mailbox one-click POST.
func (h *Handlers) SitePublicUnsubscribesPerform(ctx context.Context, params siteapi.SitePublicUnsubscribesPerformParams) (siteapi.SitePublicUnsubscribesPerformRes, error) {
	target, err := h.tracker.DecodeUnsub(params.Token)
	if err != nil {
		v := problem(http.StatusBadRequest, "invalid token")
		return &v, nil
	}
	if err := consent.RecordUnsubscribe(ctx, h.bus, target); err != nil {
		return nil, fmt.Errorf("record unsubscribe: %w", err)
	}
	return &siteapi.SitePublicUnsubscribesPerformNoContent{}, nil
}
