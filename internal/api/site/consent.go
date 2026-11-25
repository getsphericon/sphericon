package site

import (
	"context"
	"fmt"
	"net/http"

	"github.com/getsphericon/sphericon/internal/api/problem"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/confirmationrequests"
	"github.com/getsphericon/sphericon/internal/domain/sending/messagetoken"
	"github.com/getsphericon/sphericon/internal/kernel/clientip"
)

// SitePublicConfirmationsPerform is the double opt-in confirmation page's button
// (ADR 0013): the one place a confirmation is performed.
func (h *Handlers) SitePublicConfirmationsPerform(ctx context.Context, params siteapi.SitePublicConfirmationsPerformParams) error {
	target, err := h.tracker.DecodeConfirm(params.Token)
	if messagetoken.IsExpired(err) {
		// Kept by hand: no refusal kind is a 410, the answer that offers a new link.
		return &problem.Error{Status: http.StatusGone, Detail: "confirmation link expired"}
	}
	if err != nil {
		return err
	}
	if err := h.confirmations.RecordConfirmation(ctx, h.workspaces.LinkScope(target.WorkspaceID), target, clientip.FromContext(ctx)); err != nil {
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

// SitePublicConfirmationsRead is what the confirmation page of a link shows (ADR 0013): whether it
// has expired, and its Workspace's name and Brand kit as they stand.
func (h *Handlers) SitePublicConfirmationsRead(ctx context.Context, params siteapi.SitePublicConfirmationsReadParams) (*siteapi.SiteConfirmationPage, error) {
	target, expired, err := h.tracker.DecodeConfirmLink(params.Token)
	if err != nil {
		return nil, err
	}
	page, err := h.confirmationRequests.ReadPage(ctx, h.workspaces.LinkScope(target.WorkspaceID), expired)
	if err != nil {
		return nil, err
	}
	res := mapper.ConfirmationPageToResource(page)
	return &res, nil
}

// SitePublicConfirmationsRenew is the "send a new link" of a confirmation page (ADR 0013): a
// Confirmation request the person makes, which says nothing about the address.
func (h *Handlers) SitePublicConfirmationsRenew(ctx context.Context, params siteapi.SitePublicConfirmationsRenewParams) (*siteapi.ConfirmationRequestResult, error) {
	target, _, err := h.tracker.DecodeConfirmLink(params.Token)
	if err != nil {
		return nil, err
	}
	status, err := h.confirmationRequests.AskByPerson(ctx, h.workspaces.LinkScope(target.WorkspaceID), target)
	if err != nil {
		return nil, err
	}
	return &siteapi.ConfirmationRequestResult{Status: siteConfirmationStatus(status)}, nil
}

// SiteConfirmationRequestsCreate sends a Contact a confirmation email from the console (ADR 0013):
// 202 while it is on its way, 200 when the address is confirmed already.
func (h *Handlers) SiteConfirmationRequestsCreate(ctx context.Context, params siteapi.SiteConfirmationRequestsCreateParams) (siteapi.SiteConfirmationRequestsCreateRes, error) {
	status, err := h.confirmationRequests.AskByWorkspace(ctx, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	switch status {
	case confirmationrequests.Confirmed:
		return &siteapi.SiteConfirmationRequestsCreateOK{Status: siteapi.ConfirmationRequestStatusConfirmed}, nil
	case confirmationrequests.Accepted:
	}
	return &siteapi.SiteConfirmationRequestsCreateAccepted{Status: siteapi.ConfirmationRequestStatusAccepted}, nil
}

// siteConfirmationStatus is a request's status as the contract words it.
func siteConfirmationStatus(status confirmationrequests.Status) siteapi.ConfirmationRequestStatus {
	switch status {
	case confirmationrequests.Confirmed:
		return siteapi.ConfirmationRequestStatusConfirmed
	case confirmationrequests.Accepted:
	}
	return siteapi.ConfirmationRequestStatusAccepted
}
