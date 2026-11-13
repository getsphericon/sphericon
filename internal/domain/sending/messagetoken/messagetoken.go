// Package messagetoken mints and verifies the Message tokens (GLOSSARY) the email
// open/click/unsubscribe endpoints read, and rewrites an email's HTML to route
// links through the click tracker and embed the open pixel + unsubscribe footer.
// An open or click token names the Outbound message (ADR 0053), whatever surface sent it.
//
// Tokens are signed JWTs (HS256) — we reuse the existing JWT secret rather than
// hand-rolling a MAC. Link rewriting uses the official x/net/html tokenizer, not
// regexes, so arbitrary author markup round-trips safely.
package messagetoken

import (
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/golang-jwt/jwt/v5"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/linktarget"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// confirmTTL is how long a double-opt-in confirmation link stays valid. Unlike
// the eternal unsubscribe/tracking tokens, a confirmation link expires (ADR 0013):
// stale consent should not be confirmable weeks later, and never-confirmed tokens
// become purgeable. An expired link routes the recipient to "request a new one".
const confirmTTL = 7 * 24 * time.Hour

// Tracker mints recipient tokens and builds tracking URLs against baseURL.
type Tracker struct {
	secret []byte
	// now is the Instance clock a confirmation link expires by.
	now func() time.Time
	// verifyWith are the secondary secrets (a JWT secret rotation in progress): a token that
	// does not verify under secret is checked against each, so links already mailed keep working.
	verifyWith [][]byte
	baseURL    string
}

// New builds a Tracker. baseURL is the public origin (e.g. https://sphericon.localhost);
// a trailing slash is trimmed. now is the Instance clock a confirmation link expires by.
func New(secret, baseURL string, now func() time.Time, verifyWith ...string) *Tracker {
	t := &Tracker{secret: []byte(secret), now: now, baseURL: strings.TrimRight(baseURL, "/")}
	for _, v := range verifyWith {
		t.verifyWith = append(t.verifyWith, []byte(v))
	}
	return t
}

// Origin is the public origin the Tracker builds its links on, without a trailing slash.
func (t *Tracker) Origin() string { return t.baseURL }

// OnHost returns a Tracker that signs with the same secret but builds every link on
// https://<host>, the Tracking domain of one message's Sending domain (ADR 0038).
func (t *Tracker) OnHost(host string) *Tracker {
	return &Tracker{secret: t.secret, verifyWith: t.verifyWith, baseURL: "https://" + host}
}

// The tokens' claims are typed: an id travels as its public form (publicid), since a token is
// signed, not encrypted, and every recipient of a link can read it.
type messageClaims struct {
	jwt.RegisteredClaims

	WorkspaceID publicid.ID `json:"ws"`
	MessageID   publicid.ID `json:"mid"`
}

// Message is the Outbound message an open or a click token names, with its Workspace: a Tracking
// host refuses another Workspace's token by the claim alone, so a link still answers once message
// retention has deleted its message (ADR 0038, ADR 0143).
type Message struct {
	WorkspaceID, ID int64
}

// clickClaims bind one link of one Outbound message: the destination is signed, so a click URL
// cannot be pointed anywhere the sender did not put it.
type clickClaims struct {
	jwt.RegisteredClaims

	WorkspaceID publicid.ID `json:"ws"`
	MessageID   publicid.ID `json:"mid"`
	Destination string      `json:"dest"`
}

type unsubClaims struct {
	jwt.RegisteredClaims

	Channel     string      `json:"ch"`
	Source      string      `json:"src"`
	Destination string      `json:"dest"`
	WorkspaceID publicid.ID `json:"ws"`
	ContactID   publicid.ID `json:"cid"`
	BroadcastID publicid.ID `json:"bid"`
	// MessageID is absent when the opt-out names no received message.
	MessageID publicid.ID `json:"mid,omitempty"`
}

type confirmClaims struct {
	jwt.RegisteredClaims

	Destination string      `json:"dest"`
	WorkspaceID publicid.ID `json:"ws"`
	ContactID   publicid.ID `json:"cid"`
}

func (t *Tracker) sign(claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *Tracker) parse(token string, claims jwt.Claims, opts ...jwt.ParserOption) error {
	opts = append([]jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(t.now)}, opts...)
	var err error
	for _, secret := range append([][]byte{t.secret}, t.verifyWith...) {
		if _, err = jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return secret, nil }, opts...); err == nil {
			return nil
		}
	}
	return err
}

// Token mints a signed token naming an Outbound message (open pixel).
func (t *Tracker) Token(m Message) (string, error) {
	return t.sign(&messageClaims{WorkspaceID: publicid.ID(m.WorkspaceID), MessageID: publicid.ID(m.ID)})
}

// Decode validates an open token and returns the Outbound message it names. A click token
// carries a destination and is refused: each link kind has its own token.
func (t *Tracker) Decode(token string) (Message, error) {
	var c clickClaims
	if err := t.parse(token, &c); err != nil {
		return Message{}, err
	}
	if c.MessageID == 0 || c.WorkspaceID == 0 {
		return Message{}, errors.New("tracking: missing mid or ws claim")
	}
	if c.Destination != "" {
		return Message{}, errors.New("tracking: a click token is not an open token")
	}
	return Message{WorkspaceID: int64(c.WorkspaceID), ID: int64(c.MessageID)}, nil
}

// clickToken mints a signed token for one link of an Outbound message.
func (t *Tracker) clickToken(m Message, dest string) (string, error) {
	return t.sign(&clickClaims{WorkspaceID: publicid.ID(m.WorkspaceID), MessageID: publicid.ID(m.ID), Destination: dest})
}

// DecodeClick validates a click token and returns the Outbound message it names and the
// destination it carries. An open token (pixel) has no destination and is refused, as is any
// destination that is not an absolute http(s) URL.
func (t *Tracker) DecodeClick(token string) (Message, string, error) {
	var c clickClaims
	if err := t.parse(token, &c); err != nil {
		return Message{}, "", err
	}
	if c.MessageID == 0 || c.WorkspaceID == 0 {
		return Message{}, "", errors.New("tracking: missing mid or ws claim")
	}
	if !Trackable(c.Destination) {
		return Message{}, "", errors.New("tracking: missing or invalid dest claim")
	}
	return Message{WorkspaceID: int64(c.WorkspaceID), ID: int64(c.MessageID)}, c.Destination, nil
}

// unsubToken mints a signed scoped-unsubscribe token.
func (t *Tracker) unsubToken(target linktarget.Unsubscribe) (string, error) {
	return t.sign(&unsubClaims{
		Channel:     string(target.Channel),
		Source:      target.Source,
		Destination: target.Destination,
		WorkspaceID: publicid.ID(target.WorkspaceID),
		ContactID:   publicid.ID(target.ContactID),
		BroadcastID: publicid.ID(target.BroadcastID),
		MessageID:   publicid.ID(target.OutboundMessageID),
	})
}

// ErrLinkInvalid is a link token that is malformed, signed with another key or missing a claim
// its link needs: a mailed link that does not stand. A confirmation link past its TTL is one too,
// and IsExpired tells it apart.
var ErrLinkInvalid = validation.LinkInvalid("tracking: invalid link token", i18n.ErrorsTrackingLinkInvalid)

// DecodeUnsub validates a scoped-unsubscribe token and returns its target; a token that does
// not stand is ErrLinkInvalid.
func (t *Tracker) DecodeUnsub(token string) (linktarget.Unsubscribe, error) {
	var c unsubClaims
	if err := t.parse(token, &c); err != nil {
		return linktarget.Unsubscribe{}, fmt.Errorf("%w: %w", ErrLinkInvalid, err)
	}
	if c.Source == "" {
		return linktarget.Unsubscribe{}, ErrLinkInvalid.With("missing src claim")
	}
	switch channels.Channel(c.Channel) {
	case channels.Email, channels.Telegram, channels.SMS:
	default:
		return linktarget.Unsubscribe{}, ErrLinkInvalid.With("missing or unknown ch claim")
	}
	if c.WorkspaceID == 0 {
		return linktarget.Unsubscribe{}, ErrLinkInvalid.With("missing ws claim")
	}
	return linktarget.Unsubscribe{
		Channel: channels.Channel(c.Channel), Source: c.Source, Destination: c.Destination,
		WorkspaceID: int64(c.WorkspaceID), ContactID: int64(c.ContactID), BroadcastID: int64(c.BroadcastID),
		OutboundMessageID: int64(c.MessageID),
	}, nil
}

// confirmToken mints a signed, expiring confirmation token; exp is a real JWT
// numeric date so DecodeConfirm can reject a stale link.
func (t *Tracker) confirmToken(target linktarget.Confirmation) (string, error) {
	return t.sign(&confirmClaims{
		ExpiresAt:   jwt.NewNumericDate(t.now().Add(confirmTTL)),
		Destination: target.Destination,
		WorkspaceID: publicid.ID(target.WorkspaceID),
		ContactID:   publicid.ID(target.ContactID),
	})
}

// IsExpired reports whether err from DecodeConfirm means the link is past its TTL,
// as opposed to malformed or signed with another key: an expired link can be
// replaced by a new one, an invalid one is simply not ours.
func IsExpired(err error) bool {
	return errors.Is(err, jwt.ErrTokenExpired)
}

// DecodeConfirm validates a confirmation token and returns its target. It
// rejects a token whose exp has passed (and requires exp to be present), so an
// expired link cannot confirm — the endpoint surfaces that as "request a new one". A token
// that does not stand, expired or not, is ErrLinkInvalid.
func (t *Tracker) DecodeConfirm(token string) (linktarget.Confirmation, error) {
	var c confirmClaims
	if err := t.parse(token, &c, jwt.WithExpirationRequired()); err != nil {
		return linktarget.Confirmation{}, fmt.Errorf("%w: %w", ErrLinkInvalid, err)
	}
	if c.Destination == "" {
		return linktarget.Confirmation{}, ErrLinkInvalid.With("missing dest claim")
	}
	if c.WorkspaceID == 0 {
		return linktarget.Confirmation{}, ErrLinkInvalid.With("missing ws claim")
	}
	return linktarget.Confirmation{Destination: c.Destination, WorkspaceID: int64(c.WorkspaceID), ContactID: int64(c.ContactID)}, nil
}

// OpenURL is the 1x1 pixel URL that records an open.
func (t *Tracker) OpenURL(token string) string {
	return t.baseURL + paths.LinkOpen + token
}

// ClickURL wraps a destination link so a click is recorded before redirecting. The
// destination travels inside the signed token, never as a bare parameter.
func (t *Tracker) ClickURL(m Message, dest string) (string, error) {
	token, err := t.clickToken(m, dest)
	if err != nil {
		return "", err
	}
	return t.baseURL + paths.LinkClick + token, nil
}

// ShortURL is the short tracked link of a Telegram message's link or button (ADR 0083): the token
// names the URL stored with the Outbound message, so nothing of the destination travels in it.
func (t *Tracker) ShortURL(token string) string {
	return t.baseURL + paths.ShortLink + token
}

// UnsubscribeURL is the public link that records the scoped opt-out in target.
func (t *Tracker) UnsubscribeURL(target linktarget.Unsubscribe) (string, error) {
	token, err := t.unsubToken(target)
	if err != nil {
		return "", err
	}
	return t.baseURL + paths.LinkUnsubscribe + token, nil
}

// ConfirmURL is the public double-opt-in link (ADR 0013). Like UnsubscribeURL the
// GET is safe: it renders the SPA confirmation page, and the confirmation is
// recorded only when the recipient POSTs back (scanner-safe, deliberate consent).
func (t *Tracker) ConfirmURL(target linktarget.Confirmation) (string, error) {
	token, err := t.confirmToken(target)
	if err != nil {
		return "", err
	}
	return t.baseURL + paths.LinkConfirm + token, nil
}

// UnsubscribeFooter is the email's unsubscribe footer, scoped to target. Shared by
// the broadcast Rewrite and the automation send step (which appends it directly).
// postalAddress is the sending workspace's physical address (CAN-SPAM); when
// non-empty it is escaped and appended below the unsubscribe line (its own
// newlines become <br>). An empty address renders the footer unchanged — the
// address is a non-gating nudge, not a send-blocker.
func (t *Tracker) UnsubscribeFooter(target linktarget.Unsubscribe, postalAddress string) (string, error) {
	url, err := t.UnsubscribeURL(target)
	if err != nil {
		return "", err
	}
	var address string
	if addr := strings.TrimSpace(postalAddress); addr != "" {
		address = "<br>" + strings.ReplaceAll(stdhtml.EscapeString(addr), "\n", "<br>")
	}
	return fmt.Sprintf(
		`<p style="font-size:12px;color:#888888;margin-top:24px">`+
			`If you no longer wish to receive these emails, `+
			`<a href="%s">unsubscribe</a>.%s</p>`,
		url, address,
	), nil
}

// Instrument routes a body's http(s) links through the click tracker (mid token) and appends
// the open pixel, nothing else: a Transactional send that asks for tracking uses it as is. On a
// tokenizer error it falls back to the original body plus the pixel, so a send is never blocked
// by odd markup.
func (t *Tracker) Instrument(body string, m Message) (string, error) {
	token, err := t.Token(m)
	if err != nil {
		return body, err
	}
	rewritten, err := t.rewriteLinks(body, m)
	if err != nil {
		rewritten = body
	}
	pixel := fmt.Sprintf(`<img src="%s" width="1" height="1" alt="" style="display:none">`, t.OpenURL(token))
	return rewritten + pixel, nil
}

// Rewrite prepares a marketing body for one Outbound message: Instrument, plus the
// unsubscribe footer (scoped to unsub, carrying the workspace's CAN-SPAM postal address).
func (t *Tracker) Rewrite(body string, m Message, unsub linktarget.Unsubscribe, postalAddress string) (string, error) {
	instrumented, err := t.Instrument(body, m)
	if err != nil {
		return body, err
	}
	footer, err := t.UnsubscribeFooter(unsub, postalAddress)
	if err != nil {
		return body, err
	}
	return instrumented + footer, nil
}

// rewriteLinks routes the href of every <a> start tag through the click tracker. Only a tag whose
// href it changes is serialized again; every other token goes out as its raw bytes, so the body
// stays as it was written (ADR 0118): a token serialized again would escape the raw text of
// <style> and <script> ('Arial' to &#39;Arial&#39;) and turn entities into characters.
func (t *Tracker) rewriteLinks(body string, m Message) (string, error) {
	z := html.NewTokenizer(strings.NewReader(body))
	var sb strings.Builder
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return sb.String(), nil
			}
			return "", z.Err()
		}
		if tt != html.StartTagToken {
			sb.Write(z.Raw())
			continue
		}
		raw := string(z.Raw())
		tok := z.Token()
		tracked := false
		if tok.DataAtom == atom.A {
			for i, a := range tok.Attr {
				if a.Key == "href" && Trackable(a.Val) {
					link, err := t.ClickURL(m, a.Val)
					if err != nil {
						return "", err
					}
					tok.Attr[i].Val = link
					tracked = true
				}
			}
		}
		if tracked {
			sb.WriteString(tok.String())
		} else {
			sb.WriteString(raw)
		}
	}
}

// Trackable reports whether a link should be routed through the click tracker.
// Only absolute http(s) links are wrapped; mailto/tel/anchors are left alone.
func Trackable(href string) bool {
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")
}
