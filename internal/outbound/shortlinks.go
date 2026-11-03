package outbound

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/trackedlink"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/telegramrender"
	"github.com/getsphericon/sphericon/internal/tracking"
)

// tokenAttempts bounds the draws of a short token that meets an existing one. At 64 random bits a
// second draw is already a freak, so exhausting them means the storage is failing.
const tokenAttempts = 4

// shortLinks turns the links of a rendered Telegram message into short tracked links (ADR 0083):
// `https://<Tracking domain or Instance host>/c/<token>`, the token stored with the Outbound message
// and the URL it stands for. It runs during render, after the claim, so a retry of the same
// message finds its tokens again instead of minting new ones.
type shortLinks struct {
	s         *ent.Scoped
	tracker   *tracking.Tracker
	messageID int64
	// keep is a URL that stays as it is: the message's own unsubscribe link, whose click must not
	// count as engagement.
	keep string
}

// shortLinksFor is the rewriter of the links of one message, nil when it has none to track: no
// Tracker, or a message that is no real send (messageID 0: a Test send or a preview).
func (m *Module) shortLinksFor(ctx context.Context, s *ent.Scoped, messageID int64, keep string) (*shortLinks, error) {
	if m.tracker == nil || messageID == 0 {
		return nil, nil
	}
	tr, err := m.trackerFor(ctx, s)
	if err != nil {
		return nil, err
	}
	return &shortLinks{s: s, tracker: tr, messageID: messageID, keep: keep}, nil
}

// text rewrites the href of every link of the rendered text.
func (l *shortLinks) text(ctx context.Context, markup string) (string, error) {
	if l == nil {
		return markup, nil
	}
	return telegramrender.RewriteLinks(markup, func(href string) (string, error) { return l.short(ctx, href) })
}

// buttons rewrites the URL of every link button.
func (l *shortLinks) buttons(ctx context.Context, buttons []messaging.TelegramButton) ([]messaging.TelegramButton, error) {
	if l == nil {
		return buttons, nil
	}
	out := make([]messaging.TelegramButton, len(buttons))
	for i, b := range buttons {
		url, err := l.short(ctx, b.URL)
		if err != nil {
			return nil, err
		}
		out[i] = messaging.TelegramButton{Label: b.Label, URL: url}
	}
	return out, nil
}

// short is the short link of url for this message: the one it already has, else a new one. A URL
// that is not absolute http(s) is left as it is.
func (l *shortLinks) short(ctx context.Context, url string) (string, error) {
	if url == l.keep || !tracking.Trackable(url) {
		return url, nil
	}
	row, err := l.s.TrackedLink().Query().
		Where(trackedlink.OutboundMessageID(l.messageID), trackedlink.URL(url)).
		First(ctx)
	if ent.IsNotFound(err) {
		row, err = l.create(ctx, url)
	}
	if err != nil {
		return "", fmt.Errorf("outbound: short link: %w", err)
	}
	return l.tracker.ShortURL(row.Token), nil
}

func (l *shortLinks) create(ctx context.Context, url string) (*ent.TrackedLink, error) {
	var err error
	for range tokenAttempts {
		var token string
		if token, err = newShortToken(); err != nil {
			return nil, err
		}
		var row *ent.TrackedLink
		if row, err = l.s.TrackedLink().Create().SetOutboundMessageID(l.messageID).SetToken(token).SetURL(url).Save(ctx); err == nil {
			return row, nil
		}
		if !ent.IsConstraintError(err) {
			break
		}
	}
	return nil, err
}

// newShortToken is 8 random bytes in URL-safe base64 without padding: 11 characters.
func newShortToken() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate short token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
