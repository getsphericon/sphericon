package site

import (
	"context"
	"fmt"
	"net/http"

	"github.com/getsphericon/sphericon/internal/api/problem"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/clientip"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// SitePublicConfirmationsPerform is the double opt-in confirmation page's button
// (ADR 0013): the one place a confirmation is performed.
func (h *Handlers) SitePublicConfirmationsPerform(ctx context.Context, params siteapi.SitePublicConfirmationsPerformParams) error {
	target, err := h.tracker.DecodeConfirm(params.Token)
	if tracking.IsExpired(err) {
		// Kept by hand: no refusal kind is a 410, the answer that offers a new link.
		return &problem.Error{Status: http.StatusGone, Detail: "confirmation link expired"}
	}
	if err != nil {
		return err
	}
	if err := confirmations.RecordConfirmation(ctx, h.bus, h.workspaces.LinkScope(target.WorkspaceID), target, clientip.FromContext(ctx)); err != nil {
		return fmt.Errorf("record confirmation: %w", err)
	}
	return nil
}

// SitePublicUnsubscribesPerform is the unsubscribe page's button (ADR 0012): the
// same effect as POST /e/u/{token}, which stays for the mailbox one-click POST.
func (h *Handlers) SitePublicUnsubscribesPerform(ctx context.Context, params siteapi.SitePublicUnsubscribesPerformParams) error {
	target, err := h.tracker.DecodeUnsub(params.Token)
	if err != nil {
		return err
	}
	if err := h.unsubscribe.RecordLink(ctx, h.workspaces.LinkScope(target.WorkspaceID), target); err != nil {
		return fmt.Errorf("record unsubscribe: %w", err)
	}
	return nil
}
