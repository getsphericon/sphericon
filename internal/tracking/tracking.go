// Package tracking mints and verifies the per-recipient tokens used by the email
// open/click/unsubscribe endpoints, and rewrites a broadcast's HTML to route
// links through the click tracker and embed the open pixel + unsubscribe footer.
//
// Tokens are signed JWTs (HS256) — we reuse the existing JWT secret rather than
// hand-rolling a MAC. Link rewriting uses the official x/net/html tokenizer, not
// regexes, so arbitrary author markup round-trips safely.
package tracking

import (
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/net/html"
)

// confirmTTL is how long a double-opt-in confirmation link stays valid. Unlike
// the eternal unsubscribe/tracking tokens, a confirmation link expires (ADR 0013):
// stale consent should not be confirmable weeks later, and never-confirmed tokens
// become purgeable. An expired link routes the recipient to "request a new one".
const confirmTTL = 7 * 24 * time.Hour

// Tracker mints recipient tokens and builds tracking URLs against baseURL.
type Tracker struct {
	secret []byte
	// verifyWith are the secondary secrets (a JWT secret rotation in progress): a token that
	// does not verify under secret is checked against each, so links already mailed keep working.
	verifyWith [][]byte
	baseURL    string
}

// New builds a Tracker. baseURL is the public origin (e.g. https://sphericon.localhost);
// a trailing slash is trimmed.
func New(secret, baseURL string, verifyWith ...string) *Tracker {
	t := &Tracker{secret: []byte(secret), baseURL: strings.TrimRight(baseURL, "/")}
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

// UnsubTarget is the self-describing payload an unsubscribe link carries. The
// opt-out is destination-keyed (ADR 0001), so the link records against the
// destination directly — no contact-row lookup, and it survives the contact being
// deleted between send and click. BroadcastID is set only for broadcast sends
// (for per-broadcast attribution); Source is the sending-source string the opt-out
// is scoped to ("broadcasts" / "automation:<id>" / "everything").
type UnsubTarget struct {
	Source      string
	Destination string
	WorkspaceID int64
	ContactID   int64
	BroadcastID int64
}

// The tokens' claims are typed: int64 ids travel as JSON strings (`,string`) so
// they never pass through float64 and lose precision above 2^53.
type recipientClaims struct {
	jwt.RegisteredClaims
	RecipientID int64 `json:"rid,string"`
}

// clickClaims bind one link of one recipient: the destination is signed, so a click URL
// cannot be pointed anywhere the sender did not put it.
type clickClaims struct {
	jwt.RegisteredClaims
	RecipientID int64  `json:"rid,string"`
	Destination string `json:"dest"`
}

type unsubClaims struct {
	jwt.RegisteredClaims
	Source      string `json:"src"`
	Destination string `json:"dest"`
	WorkspaceID int64  `json:"ws,string"`
	ContactID   int64  `json:"cid,string"`
	BroadcastID int64  `json:"bid,string"`
}

type confirmClaims struct {
	jwt.RegisteredClaims
	Destination string `json:"dest"`
	WorkspaceID int64  `json:"ws,string"`
	ContactID   int64  `json:"cid,string"`
}

func (t *Tracker) sign(claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

func (t *Tracker) parse(token string, claims jwt.Claims, opts ...jwt.ParserOption) error {
	opts = append([]jwt.ParserOption{jwt.WithValidMethods([]string{"HS256"})}, opts...)
	var err error
	for _, secret := range append([][]byte{t.secret}, t.verifyWith...) {
		if _, err = jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return secret, nil }, opts...); err == nil {
			return nil
		}
	}
	return err
}

// Token mints a signed token identifying a broadcast recipient (open pixel).
func (t *Tracker) Token(recipientID int64) (string, error) {
	return t.sign(&recipientClaims{RecipientID: recipientID})
}

// Decode validates an open token and returns the recipient id it carries. A click token
// carries a destination and is refused: each link kind has its own token.
func (t *Tracker) Decode(token string) (int64, error) {
	var c clickClaims
	if err := t.parse(token, &c); err != nil {
		return 0, err
	}
	if c.RecipientID == 0 {
		return 0, errors.New("tracking: missing rid claim")
	}
	if c.Destination != "" {
		return 0, errors.New("tracking: a click token is not an open token")
	}
	return c.RecipientID, nil
}

// clickToken mints a signed token for one link of a recipient's email.
func (t *Tracker) clickToken(recipientID int64, dest string) (string, error) {
	return t.sign(&clickClaims{RecipientID: recipientID, Destination: dest})
}

// DecodeClick validates a click token and returns the recipient id and the destination it
// carries. A recipient token (open pixel) has no destination and is refused, as is any
// destination that is not an absolute http(s) URL.
func (t *Tracker) DecodeClick(token string) (int64, string, error) {
	var c clickClaims
	if err := t.parse(token, &c); err != nil {
		return 0, "", err
	}
	if c.RecipientID == 0 {
		return 0, "", errors.New("tracking: missing rid claim")
	}
	if !trackable(c.Destination) {
		return 0, "", errors.New("tracking: missing or invalid dest claim")
	}
	return c.RecipientID, c.Destination, nil
}

// unsubToken mints a signed scoped-unsubscribe token.
func (t *Tracker) unsubToken(target UnsubTarget) (string, error) {
	return t.sign(&unsubClaims{
		Source:      target.Source,
		Destination: target.Destination,
		WorkspaceID: target.WorkspaceID,
		ContactID:   target.ContactID,
		BroadcastID: target.BroadcastID,
	})
}

// DecodeUnsub validates a scoped-unsubscribe token and returns its target.
func (t *Tracker) DecodeUnsub(token string) (UnsubTarget, error) {
	var c unsubClaims
	if err := t.parse(token, &c); err != nil {
		return UnsubTarget{}, err
	}
	if c.Source == "" {
		return UnsubTarget{}, errors.New("tracking: missing src claim")
	}
	if c.WorkspaceID == 0 {
		return UnsubTarget{}, errors.New("tracking: missing ws claim")
	}
	return UnsubTarget{Source: c.Source, Destination: c.Destination, WorkspaceID: c.WorkspaceID, ContactID: c.ContactID, BroadcastID: c.BroadcastID}, nil
}

// ConfirmTarget is the payload a double-opt-in confirmation link carries (ADR
// 0013). Like the opt-out, the confirmation is destination-keyed and not
// source-scoped: confirming an address makes it mailable across sources, so the
// link records against the destination directly (no contact-row lookup) and
// survives the contact being deleted between send and click.
type ConfirmTarget struct {
	Destination string
	WorkspaceID int64
	ContactID   int64
}

// confirmToken mints a signed, expiring confirmation token; exp is a real JWT
// numeric date so DecodeConfirm can reject a stale link.
func (t *Tracker) confirmToken(target ConfirmTarget) (string, error) {
	return t.sign(&confirmClaims{
		ExpiresAt:   jwt.NewNumericDate(time.Now().Add(confirmTTL)),
		Destination: target.Destination,
		WorkspaceID: target.WorkspaceID,
		ContactID:   target.ContactID,
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
// expired link cannot confirm — the endpoint surfaces that as "request a new one".
func (t *Tracker) DecodeConfirm(token string) (ConfirmTarget, error) {
	var c confirmClaims
	if err := t.parse(token, &c, jwt.WithExpirationRequired()); err != nil {
		return ConfirmTarget{}, err
	}
	if c.Destination == "" {
		return ConfirmTarget{}, errors.New("tracking: missing dest claim")
	}
	if c.WorkspaceID == 0 {
		return ConfirmTarget{}, errors.New("tracking: missing ws claim")
	}
	return ConfirmTarget{Destination: c.Destination, WorkspaceID: c.WorkspaceID, ContactID: c.ContactID}, nil
}

// OpenURL is the 1x1 pixel URL that records an open.
func (t *Tracker) OpenURL(token string) string {
	return t.baseURL + "/e/o/" + token
}

// ClickURL wraps a destination link so a click is recorded before redirecting. The
// destination travels inside the signed token, never as a bare parameter.
func (t *Tracker) ClickURL(recipientID int64, dest string) (string, error) {
	token, err := t.clickToken(recipientID, dest)
	if err != nil {
		return "", err
	}
	return t.baseURL + "/e/c/" + token, nil
}

// UnsubscribeURL is the public link that records the scoped opt-out in target.
func (t *Tracker) UnsubscribeURL(target UnsubTarget) (string, error) {
	token, err := t.unsubToken(target)
	if err != nil {
		return "", err
	}
	return t.baseURL + "/e/u/" + token, nil
}

// ConfirmURL is the public double-opt-in link (ADR 0013). Like UnsubscribeURL the
// GET is safe: it renders the SPA confirmation page, and the confirmation is
// recorded only when the recipient POSTs back (scanner-safe, deliberate consent).
func (t *Tracker) ConfirmURL(target ConfirmTarget) (string, error) {
	token, err := t.confirmToken(target)
	if err != nil {
		return "", err
	}
	return t.baseURL + "/e/confirm/" + token, nil
}

// UnsubscribeFooter is the email's unsubscribe footer, scoped to target. Shared by
// the broadcast Rewrite and the automation send step (which appends it directly).
// postalAddress is the sending workspace's physical address (CAN-SPAM); when
// non-empty it is escaped and appended below the unsubscribe line (its own
// newlines become <br>). An empty address renders the footer unchanged — the
// address is a non-gating nudge, not a send-blocker.
func (t *Tracker) UnsubscribeFooter(target UnsubTarget, postalAddress string) (string, error) {
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

// Rewrite prepares a broadcast body for a recipient: it routes http(s) links
// through the click tracker (rid token), appends the open pixel, and adds the
// unsubscribe footer (scoped to unsub, carrying the workspace's CAN-SPAM postal
// address). On a tokenizer error it falls back to the original body plus pixel
// and footer so a send is never blocked by odd markup.
func (t *Tracker) Rewrite(body string, recipientID int64, unsub UnsubTarget, postalAddress string) (string, error) {
	token, err := t.Token(recipientID)
	if err != nil {
		return body, err
	}

	rewritten, err := t.rewriteLinks(body, recipientID)
	if err != nil {
		rewritten = body
	}

	footer, err := t.UnsubscribeFooter(unsub, postalAddress)
	if err != nil {
		return body, err
	}
	pixel := fmt.Sprintf(`<img src="%s" width="1" height="1" alt="" style="display:none">`, t.OpenURL(token))
	return rewritten + pixel + footer, nil
}

func (t *Tracker) rewriteLinks(body string, recipientID int64) (string, error) {
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
		tok := z.Token()
		if tt == html.StartTagToken && tok.Data == "a" {
			for i, a := range tok.Attr {
				if a.Key == "href" && trackable(a.Val) {
					link, err := t.ClickURL(recipientID, a.Val)
					if err != nil {
						return "", err
					}
					tok.Attr[i].Val = link
				}
			}
		}
		sb.WriteString(tok.String())
	}
}

// trackable reports whether a link should be routed through the click tracker.
// Only absolute http(s) links are wrapped; mailto/tel/anchors are left alone.
func trackable(href string) bool {
	return strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://")
}
