package outbound

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/bindings"
)

// StartLinkTag is the merge tag of a Telegram start link (ADR 0076): on every Channel, for the
// Workspace's default Telegram Integration.
const StartLinkTag = "telegram_start_url"

// PreviewStartLink is what the tag renders as in a preview, which issues no token.
const PreviewStartLink = "https://t.me/your_bot?start=preview"

// StartLinks issues the start link the merge tag renders; bindings.Module is it.
type StartLinks interface {
	IssueStartLink(ctx context.Context, s *ent.Scoped, contactID int64, integrationID *int64) (bindings.StartLink, error)
}

// WithStartLinks gives the module the issuer of the Start tokens of {{ telegram_start_url }}.
// Without it the tag renders empty.
func WithStartLinks(l StartLinks) Option { return func(m *Module) { m.startLinks = l } }

// startLinkBinding binds StartLinkTag for a message whose content is texts, or binds nothing when
// none of them uses the tag: a token is issued only for content that carries it. The tag renders
// empty when the Workspace has no enabled default Telegram Integration or the message has no
// Contact. A message that is no real send (messageID 0: a Test send) gets the placeholder and
// issues nothing. Each call issues a fresh token, so a render retried after a failed attempt
// leaves the earlier one to lapse. An error is the storage's, not the message's, and is retried.
func (m *Module) startLinkBinding(ctx context.Context, s *ent.Scoped, req Request, messageID int64, texts ...string) (map[string]any, error) {
	used := false
	for _, t := range texts {
		used = used || strings.Contains(t, StartLinkTag)
	}
	if !used {
		return nil, nil
	}
	switch {
	case messageID == 0:
		return map[string]any{StartLinkTag: PreviewStartLink}, nil
	case m.startLinks == nil || req.contactID() == 0:
		return map[string]any{StartLinkTag: ""}, nil
	}
	link, err := m.startLinks.IssueStartLink(ctx, s, req.contactID(), nil)
	switch {
	case errors.Is(err, bindings.ErrNoIntegration), errors.Is(err, bindings.ErrBotOff):
		return map[string]any{StartLinkTag: ""}, nil
	case err != nil:
		return nil, fmt.Errorf("outbound: issue start link: %w", err)
	}
	return map[string]any{StartLinkTag: link.URL}, nil
}
