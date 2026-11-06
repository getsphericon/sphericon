// Package bindings is where a Telegram chat becomes a Contact's: the Start token a start link
// carries and the Binding a `/start <token>` makes (ADR 0075, ADR 0076). A Binding is made only by
// what the bot receives, never written through an API; the token is issued for a Contact through
// the APIs, and later for a Visitor by the tracker or a merge tag.
package bindings

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/binding"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/starttoken"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/contactlock"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// TokenTTL is how long a start link works: a link waits in an inbox for weeks (ADR 0076).
const TokenTTL = 30 * 24 * time.Hour

var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = validation.NotFound("bindings: contact not found", i18n.ErrorsContactNotFound)
	// ErrNoIntegration: no Integration to issue the link for: the Workspace has no default
	// Telegram Integration.
	ErrNoIntegration = validation.RefuseField("integrationId", "bindings: no default Telegram integration", i18n.ErrorsTelegramStartNoIntegration)
	// ErrNotTelegram: the named Integration is not a Telegram bot.
	ErrNotTelegram = validation.RefuseField("integrationId", "bindings: not a Telegram integration", i18n.ErrorsTelegramStartNotTelegram)
	// ErrBotOff: the named bot holds no webhook (disabled or archived), so a link to it would
	// bind nothing.
	ErrBotOff = validation.RefuseField("integrationId", "bindings: the bot is off", i18n.ErrorsTelegramStartBotOff)
)

// tokenPattern is what a Start token may be: Telegram's limit on a deep-link parameter.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Module issues start links and lists Bindings.
type Module struct {
	integ *integrations.Module
	now   func() time.Time
}

// New builds the module; now is the clock a token's expiry is counted from.
func New(integ *integrations.Module, now func() time.Time) *Module {
	return &Module{integ: integ, now: now}
}

// StartLink is a start link and when it lapses.
type StartLink struct {
	URL       string
	ExpiresAt time.Time
}

// IssueStartLink issues a single-use Start token for the Contact and returns the link
// `https://t.me/<bot>?start=<token>` that carries it. Only the token's hash is stored. The bot is
// the named Integration, or the default Telegram one when integrationID is nil.
func (m *Module) IssueStartLink(ctx context.Context, s *ent.Scoped, contactID int64, integrationID *int64) (StartLink, error) {
	exists, err := s.Contact().Query().Where(contact.ID(contactID)).Exist(ctx)
	if err != nil {
		return StartLink{}, fmt.Errorf("find contact: %w", err)
	}
	if !exists {
		return StartLink{}, ErrContactNotFound
	}
	return m.issue(ctx, s, integrationID, func(c *ent.StartTokenScopedCreate) { c.SetContactID(contactID) })
}

// IssueVisitorStartLink issues a single-use Start token for the Visitor with the tracker's id
// visitorID, creating the Visitor when it has not been seen, on the default Telegram Integration.
// The chat that starts with it binds to the Visitor and reaches its Contact once Identify resolves
// it (ADR 0076). ErrNoIntegration and ErrBotOff when there is no bot to link to.
func (m *Module) IssueVisitorStartLink(ctx context.Context, s *ent.Scoped, visitorID string) (StartLink, error) {
	vis, err := findOrCreateVisitor(ctx, s, visitorID)
	if err != nil {
		return StartLink{}, err
	}
	return m.issue(ctx, s, nil, func(c *ent.StartTokenScopedCreate) { c.SetVisitorID(vis.ID) })
}

func findOrCreateVisitor(ctx context.Context, s *ent.Scoped, visitorID string) (*ent.Visitor, error) {
	vis, err := s.Visitor().Query().Where(visitor.VisitorID(visitorID)).First(ctx)
	if err == nil {
		return vis, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("find visitor: %w", err)
	}
	vis, err = s.Visitor().Create().SetVisitorID(visitorID).SetLastSeenAt(time.Now()).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create visitor: %w", err)
	}
	return vis, nil
}

// issue stores a Start token whose owner own sets and returns the link that carries it.
func (m *Module) issue(ctx context.Context, s *ent.Scoped, integrationID *int64, own func(*ent.StartTokenScopedCreate)) (StartLink, error) {
	row, err := m.bot(ctx, s, integrationID)
	if err != nil {
		return StartLink{}, err
	}
	cfg, err := m.integ.Redact(row)
	if err != nil || cfg.Telegram == nil {
		return StartLink{}, fmt.Errorf("open the bot of integration %d: %w", row.ID, err)
	}
	token, err := newToken()
	if err != nil {
		return StartLink{}, err
	}
	expires := m.now().Add(TokenTTL)
	create := s.StartToken().Create().
		SetIntegrationID(row.ID).
		SetTokenHash(hashOf(token)).
		SetExpiresAt(expires)
	own(create)
	if err := create.Exec(ctx); err != nil {
		return StartLink{}, fmt.Errorf("store start token: %w", err)
	}
	return StartLink{URL: "https://t.me/" + cfg.Telegram.Username + "?start=" + token, ExpiresAt: expires}, nil
}

// bot is the Integration a link is issued for: the named one, or the default Telegram one.
func (m *Module) bot(ctx context.Context, s *ent.Scoped, id *int64) (*ent.Integration, error) {
	q := s.Integration().Query().Where(integration.DiscardedAtIsNil())
	if id != nil {
		q = q.Where(integration.ID(*id))
	} else {
		q = q.Where(integration.ChannelEQ(channels.Telegram), integration.IsDefault(true))
	}
	row, err := q.Only(ctx)
	switch {
	case ent.IsNotFound(err) && id == nil:
		return nil, ErrNoIntegration
	case ent.IsNotFound(err):
		return nil, integrations.ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("find integration: %w", err)
	}
	if row.Channel != channels.Telegram {
		return nil, ErrNotTelegram
	}
	if !row.Enabled {
		return nil, ErrBotOff
	}
	return row, nil
}

// newToken is 32 random bytes in URL-safe base64 without padding: 43 characters of
// [A-Za-z0-9_-].
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate start token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashOf(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Chat is a Binding as the Contact's card shows it: the bot and since when.
type Chat struct {
	ID              int64
	IntegrationID   int64
	IntegrationName string
	// BotUsername is the bot's @username without the @-sign, "" when it cannot be read.
	BotUsername string
	BoundAt     time.Time
}

// ForContact lists the Telegram Bindings a Contact is reached through, its own and those of its
// Visitors, with the bot each is on, oldest first.
func (m *Module) ForContact(ctx context.Context, s *ent.Scoped, contactID int64) ([]Chat, error) {
	exists, err := s.Contact().Query().Where(contact.ID(contactID)).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("find contact: %w", err)
	}
	if !exists {
		return nil, ErrContactNotFound
	}
	rows, err := s.Binding().Query().
		Where(binding.Or(binding.ContactID(contactID), binding.HasVisitorWith(visitor.ContactID(contactID)))).
		WithIntegration().
		Order(binding.ByBoundAt(), binding.ByID()).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	return lo.Map(rows, func(b *ent.Binding, _ int) Chat {
		bot := b.Edges.Integration
		c := Chat{ID: b.ID, IntegrationID: bot.ID, IntegrationName: bot.Name, BoundAt: b.BoundAt}
		if cfg, err := m.integ.Redact(bot); err == nil && cfg.Telegram != nil {
			c.BotUsername = cfg.Telegram.Username
		}
		return c
	}), nil
}

// Redemption is what a `/start <token>` carries to Redeem.
type Redemption struct {
	// Destination is the sender's Telegram user id and ChatID the chat the bot answers in.
	Destination string
	ChatID      string
	// Text is the message as sent.
	Text string
	// DedupKey is the update's key (ingesthook.Tx.DedupKey): the events it records derive theirs from it.
	DedupKey string
}

// Redeem reads a message for a `/start <token>`. A token that is unused, unexpired and of this
// Integration is consumed, and the chat is bound to its owner: a new Binding, or the existing one
// moved to the token's owner (the latest owner wins). It records telegram.bound for a Contact that
// gained the chat and telegram.unbound for the Contact that lost it. Anything else (another
// message, a bare `/start`, a used, expired or other bot's token) records nothing: the bot never
// answers. Run it inside the transaction s and pub are bound to.
func Redeem(ctx context.Context, s *ent.Scoped, pub events.Publisher, bot *ent.Integration, now time.Time, r Redemption) error {
	token, ok := startToken(r.Text)
	if !ok {
		return nil
	}
	tok, err := s.StartToken().Query().
		Where(starttoken.TokenHash(hashOf(token)), starttoken.IntegrationID(bot.ID), starttoken.UsedAtIsNil(), starttoken.ExpiresAtGT(now)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find start token: %w", err)
	}
	// Single use is one conditional write: a redelivery or a second chat racing for the token
	// finds it used and records nothing.
	used, err := s.StartToken().Update().
		Where(starttoken.ID(tok.ID), starttoken.UsedAtIsNil()).
		SetUsedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("use start token: %w", err)
	}
	if used == 0 {
		return nil
	}
	if tok.ContactID != nil {
		// The owner's row is locked first, as every write about a Contact does (contactlock), so
		// an Erasure of it and this bind are ordered.
		if _, err := contactlock.Lock(ctx, s, *tok.ContactID); err != nil {
			if ent.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("lock contact: %w", err)
		}
	}
	return bind(ctx, s, pub, bot, now, tok, r)
}

// startToken is the token of a `/start <token>` message: `/start`, optionally addressed to a bot
// (`/start@name`), then one word of what a Start token may be.
func startToken(text string) (string, bool) {
	command, rest, found := strings.Cut(strings.TrimSpace(text), " ")
	if !found {
		return "", false
	}
	if name, _, _ := strings.Cut(command, "@"); name != "/start" {
		return "", false
	}
	token := strings.TrimSpace(rest)
	return token, tokenPattern.MatchString(token)
}

func bind(ctx context.Context, s *ent.Scoped, pub events.Publisher, bot *ent.Integration, now time.Time, tok *ent.StartToken, r Redemption) error {
	existing, err := s.Binding().Query().
		Where(binding.IntegrationID(bot.ID), binding.Destination(r.Destination)).
		WithVisitor().
		Only(ctx)
	switch {
	case ent.IsNotFound(err):
		create := s.Binding().Create().
			SetIntegrationID(bot.ID).
			SetChannel(binding.ChannelTelegram).
			SetDestination(r.Destination).
			SetChatID(r.ChatID).
			SetBoundAt(now).
			SetAnnounced(tok.ContactID != nil)
		if tok.ContactID != nil {
			create.SetContactID(*tok.ContactID)
		} else {
			create.SetVisitorID(*tok.VisitorID)
		}
		if err := create.Exec(ctx); err != nil {
			return fmt.Errorf("create binding: %w", err)
		}
		return announce(ctx, s, pub, bot, events.NameTelegramBound, tok.ContactID, r.DedupKey+"/bound")
	case err != nil:
		return fmt.Errorf("find binding: %w", err)
	}
	same, err := sameContact(ctx, s, existing, tok)
	if err != nil {
		return err
	}
	if same {
		// The chat stays with the Contact it reaches, whichever of its Visitors or itself the
		// token names: nobody gains or loses it, so nothing is announced.
		if existing.ChatID == r.ChatID {
			return nil
		}
		if err := s.Binding().UpdateOneID(existing.ID).SetChatID(r.ChatID).Exec(ctx); err != nil {
			return fmt.Errorf("update binding chat: %w", err)
		}
		return nil
	}
	move := s.Binding().UpdateOneID(existing.ID).SetChatID(r.ChatID).SetBoundAt(now).SetAnnounced(tok.ContactID != nil)
	if tok.ContactID != nil {
		move.SetContactID(*tok.ContactID).ClearVisitorID()
	} else {
		move.SetVisitorID(*tok.VisitorID).ClearContactID()
	}
	if err := move.Exec(ctx); err != nil {
		return fmt.Errorf("move binding: %w", err)
	}
	if err := announce(ctx, s, pub, bot, events.NameTelegramUnbound, ownerOf(existing), r.DedupKey+"/unbound"); err != nil {
		return err
	}
	return announce(ctx, s, pub, bot, events.NameTelegramBound, tok.ContactID, r.DedupKey+"/bound")
}

// Remove deletes the Binding of the Destination on bot, a chat Telegram says no longer reaches a
// person (a deactivated user, a chat that does not exist), and records telegram.unbound for the
// Contact it reached: the Binding's own, or the one its Visitor was identified as. No Binding is
// nothing to remove. dedup keys the event, so a retried removal records it once. Run it inside the
// transaction s and pub are bound to.
func Remove(ctx context.Context, s *ent.Scoped, pub events.Publisher, bot *ent.Integration, destination, dedup string) error {
	b, err := s.Binding().Query().
		Where(binding.IntegrationID(bot.ID), binding.Destination(destination)).
		WithVisitor().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find binding: %w", err)
	}
	if err := s.Binding().DeleteOneID(b.ID).Exec(ctx); err != nil {
		return fmt.Errorf("delete binding: %w", err)
	}
	return announce(ctx, s, pub, bot, events.NameTelegramUnbound, ownerOf(b), dedup)
}

// ownerOf is the Contact a Binding reaches: its own, or the one its Visitor was identified as. b
// is read WithVisitor. A Binding of a Visitor with no Contact reaches nobody.
func ownerOf(b *ent.Binding) *int64 {
	if b.ContactID != nil {
		return b.ContactID
	}
	if b.Edges.Visitor != nil {
		return b.Edges.Visitor.ContactID
	}
	return nil
}

// sameContact reports whether the token's owner and the Binding's are one and the same: the same
// Contact or Visitor, or a Visitor and the Contact it was identified as.
func sameContact(ctx context.Context, s *ent.Scoped, b *ent.Binding, tok *ent.StartToken) (bool, error) {
	if lo.FromPtr(b.ContactID) == lo.FromPtr(tok.ContactID) && lo.FromPtr(b.VisitorID) == lo.FromPtr(tok.VisitorID) {
		return true, nil
	}
	have := ownerOf(b)
	if have == nil {
		return false, nil
	}
	want := tok.ContactID
	if want == nil {
		v, err := s.Visitor().Get(ctx, *tok.VisitorID)
		if err != nil {
			return false, fmt.Errorf("read visitor: %w", err)
		}
		want = v.ContactID
	}
	return want != nil && *want == *have, nil
}

// announce records the System event of a Contact gaining or losing the chat; a Visitor with no
// Contact has none to record it for.
func announce(ctx context.Context, s *ent.Scoped, pub events.Publisher, bot *ent.Integration, action string, contactID *int64, dedup string) error {
	if contactID == nil {
		return nil
	}
	c, err := s.Contact().Get(ctx, *contactID)
	if err != nil {
		return fmt.Errorf("read contact: %w", err)
	}
	return pub.Publish(ctx, &events.BindingChange{
		Action: action, WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email),
		IntegrationID: bot.ID, DedupID: dedup,
	})
}

// Message is a message read from a Destination, whatever it says.
type Message struct {
	Destination string
	Text        string
	// DedupKey is the update's key (ingesthook.Tx.DedupKey).
	DedupKey string
}

// Receive records telegram.received for the Contact the message's Destination is Bound to on bot,
// its own or its Visitor's,
// with the text cut by events.ReceivedText. A Destination that is unknown, or Bound to a Visitor
// with no Contact, records nothing and creates no Contact. Run it inside the transaction s and pub
// are bound to.
func Receive(ctx context.Context, s *ent.Scoped, pub events.Publisher, bot *ent.Integration, m Message) error {
	b, err := s.Binding().Query().
		Where(binding.IntegrationID(bot.ID), binding.Destination(m.Destination)).
		WithVisitor().
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find binding: %w", err)
	}
	owner := ownerOf(b)
	if owner == nil {
		return nil
	}
	c, err := s.Contact().Get(ctx, *owner)
	if err != nil {
		return fmt.Errorf("read contact: %w", err)
	}
	return pub.Publish(ctx, &events.TelegramReceived{
		WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email), IntegrationID: bot.ID,
		Text: events.ReceivedText(m.Text), DedupID: m.DedupKey,
	})
}

// AnnounceVisitor records telegram.bound for c, the Contact Identify has just resolved the Visitor
// vis to, for each of the Visitor's Bindings not yet announced (ADR 0076). A Visitor that was
// another Contact's before has all of its Bindings announced to c. Run it in the transaction s and
// pub are bound to. Each Binding is announced once per Contact: a repeated Identify records nothing.
func AnnounceVisitor(ctx context.Context, s *ent.Scoped, pub events.Publisher, vis *ent.Visitor, c *ent.Contact) error {
	q := s.Binding().Query().Where(binding.VisitorID(vis.ID)).Order(binding.ByID())
	if vis.ContactID == nil || *vis.ContactID == c.ID {
		q = q.Where(binding.Announced(false))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return fmt.Errorf("list visitor bindings: %w", err)
	}
	for _, b := range rows {
		err := pub.Publish(ctx, &events.BindingChange{
			Action: events.NameTelegramBound, WorkspaceID: s.WorkspaceID(), ContactID: c.ID, Email: lo.FromPtr(c.Email),
			IntegrationID: b.IntegrationID, DedupID: fmt.Sprintf("telegram:binding:%d:contact:%d/bound", b.ID, c.ID),
		})
		if err != nil {
			return err
		}
		if err := s.Binding().UpdateOneID(b.ID).SetAnnounced(true).Exec(ctx); err != nil {
			return fmt.Errorf("mark binding announced: %w", err)
		}
	}
	return nil
}

// PruneStartTokens deletes the Start tokens that lapsed by now or were used, in every Workspace,
// and returns how many went. A lapsed or used token redeems nothing; the tracker and a merge tag
// issue a fresh one per call, so they would otherwise pile up. The one function of the package
// that takes the raw client, called by a river job: the tokens belong to no one Workspace's request.
func PruneStartTokens(ctx context.Context, client *ent.Client, now time.Time) (int, error) {
	n, err := client.StartToken.Delete().
		Where(starttoken.Or(starttoken.ExpiresAtLTE(now), starttoken.UsedAtNotNil())).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("prune start tokens: %w", err)
	}
	return n, nil
}
