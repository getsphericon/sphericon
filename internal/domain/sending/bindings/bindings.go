// Package bindings keeps the Bindings: the record that one Integration can reach one Destination
// of a Channel whose Destination is no attribute of the Contact, and whose it is, a Contact's or a
// Visitor's (ADR 0075). A Visitor's Binding reaches nobody until an Identify the customer's server
// proved hands it, on whatever Channel, to the Contact (ADR 0171, ADR 0179). On Telegram a Binding
// is a chat: the Start token a start link carries and the Binding a `/start <token>` makes
// (ADR 0076), made only by what the bot receives, never written through an API; the token is
// issued for a Contact through the APIs, and for a Visitor by the tracker.
package bindings

import (
	"context"
	"errors"
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
	"github.com/getsphericon/sphericon/internal/domain/contacts/contactlock"
	"github.com/getsphericon/sphericon/internal/domain/contacts/visitors"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/fieldname"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// TokenTTL is how long a start link works: a link waits in an inbox for weeks (ADR 0076).
const TokenTTL = 30 * 24 * time.Hour

var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = validation.NotFound("bindings: contact not found", i18n.ErrorsContactNotFound)
	// ErrNoIntegration: no Integration to issue the link for: the Workspace has no default
	// Telegram Integration.
	ErrNoIntegration = validation.RefuseField(fieldname.IntegrationID, "bindings: no default Telegram integration", i18n.ErrorsTelegramStartNoIntegration)
	// ErrIntegrationNotFound: the named Integration is not one of the Workspace's.
	ErrIntegrationNotFound = validation.NotFound("bindings: integration not found", i18n.ErrorsIntegrationNotFound)
	// ErrNotTelegram: the named Integration is not a Telegram bot.
	ErrNotTelegram = validation.RefuseField(fieldname.IntegrationID, "bindings: not a Telegram integration", i18n.ErrorsTelegramStartNotTelegram)
	// ErrBotOff: the named bot holds no webhook (disabled or archived), so a link to it would
	// bind nothing.
	ErrBotOff = validation.RefuseField(fieldname.IntegrationID, "bindings: the bot is off", i18n.ErrorsTelegramStartBotOff)
)

// tokenPattern is what a Start token may be: Telegram's limit on a deep-link parameter.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Module issues start links and lists Bindings.
type Module struct {
	integ    *integrations.Module
	visitors *visitors.Module
	now      func() time.Time
}

// New builds the module over the Visitors a start link of a Visitor binds to; now is the clock a
// token's expiry is counted from.
func New(integ *integrations.Module, visits *visitors.Module, now func() time.Time) *Module {
	return &Module{integ: integ, visitors: visits, now: now}
}

// Chats is the Telegram side of the Bindings, what the bots learn about the chats bound to them: a
// `/start <token>` that binds one, a message read from one, a chat Telegram no longer reaches, the
// Telegram users a Contact is Bound as, and the Start tokens that lapsed. The hand-over of a
// Visitor's Bindings on every Channel is Owners'.
// It records the System events of those changes in the transaction of the scope it is given, so
// that scope must be bound to a transaction of its bus (events.Bus.PublishIn). It is a role apart
// from Module: it publishes, and it reads no Integration's config.
type Chats struct {
	bus *events.Bus
	// locks takes the owner Contact's row lock before a redemption binds the chat to it.
	locks *contactlock.Module
	now   func() time.Time
}

// NewChats builds the chats' side of the Bindings over the bus their events are published on; now
// is the clock a Binding is dated and a Start token is spent and lapses by.
func NewChats(bus *events.Bus, now func() time.Time) *Chats {
	return &Chats{bus: bus, locks: contactlock.New(), now: now}
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
// The chat that starts with it binds to the Visitor and reaches no Contact until a proven Identify
// hands it over (ADR 0171).
// ErrNoIntegration and ErrBotOff when there is no bot to link to.
func (m *Module) IssueVisitorStartLink(ctx context.Context, s *ent.Scoped, visitorID string) (StartLink, error) {
	vis, err := m.visitors.EnsureIn(ctx, s, visitorID)
	if err != nil {
		return StartLink{}, err
	}
	return m.issue(ctx, s, nil, func(c *ent.StartTokenScopedCreate) { c.SetVisitorID(vis.ID) })
}

// The unique constraints of the Bindings and Start tokens (ADR 0106): no client can break them.
var _ = db.Declare(
	db.Fault("bindings_workspace_id_integration_destination",
		"a Binding is written with ON CONFLICT DO NOTHING, and a redemption that meets one goes on with it"),
	db.Fault("start_tokens_token_hash", "a Start token is random (newToken), stored by its hash"),
)

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
	token := credentials.GenerateTokenSecret()
	expires := m.now().Add(TokenTTL)
	create := s.StartToken().Create().
		SetIntegrationID(row.ID).
		SetTokenHash(credentials.HashSecret(token)).
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
		return nil, ErrIntegrationNotFound
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

// Chat is a Binding as the Contact's card shows it: the bot and since when.
type Chat struct {
	ID              int64
	IntegrationID   int64
	IntegrationName string
	// BotUsername is the bot's @username without the @-sign, "" when it cannot be read.
	BotUsername string
	BoundAt     time.Time
}

// ForContact lists the Telegram Bindings a Contact is reached through, with the bot each is on,
// oldest first.
func (m *Module) ForContact(ctx context.Context, s *ent.Scoped, contactID int64) ([]Chat, error) {
	exists, err := s.Contact().Query().Where(contact.ID(contactID)).Exist(ctx)
	if err != nil {
		return nil, fmt.Errorf("find contact: %w", err)
	}
	if !exists {
		return nil, ErrContactNotFound
	}
	rows, err := s.Binding().Query().
		Where(binding.ChannelEQ(binding.ChannelTelegram), binding.ContactID(contactID)).
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
	// DedupKey is the update's key (callbackinterpretation.Tx.DedupKey): the events it records derive theirs from it.
	DedupKey string
}

// Redeem reads a message for a `/start <token>`. A token that is unused, unexpired and of this
// Integration is consumed, and the chat is bound to its owner: a new Binding, or the existing one
// moved to the token's owner (the latest owner wins). It records telegram.bound for a Contact that
// gained the chat and telegram.unbound for the Contact that lost it. Anything else (another
// message, a bare `/start`, a used, expired or other bot's token) records nothing: the bot never
// answers. Run it inside a transaction of the bus that s is bound to.
func (c *Chats) Redeem(ctx context.Context, s *ent.Scoped, bot *ent.Integration, r Redemption) error {
	now := c.now()
	token, ok := startToken(r.Text)
	if !ok {
		return nil
	}
	tok, err := s.StartToken().Query().
		Where(starttoken.TokenHash(credentials.HashSecret(token)), starttoken.IntegrationID(bot.ID), starttoken.UsedAtIsNil(), starttoken.ExpiresAtGT(now)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find start token: %w", err)
	}
	// Single use is one conditional write: a redelivery or a second chat racing for the token
	// finds it used and records nothing.
	err = lifecycle.Latch(ctx, func(ctx context.Context) (bool, error) {
		used, err := s.StartToken().Update().
			Where(starttoken.ID(tok.ID), starttoken.UsedAtIsNil()).
			SetUsedAt(now).
			Save(ctx)
		return used > 0, err
	})
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("use start token: %w", err)
	}
	own := owner{contactID: tok.ContactID, visitorID: tok.VisitorID}
	if own.contactID != nil {
		// The owner's row is locked first, as every write about a Contact does (contactlock), so
		// an Erasure of it and this bind are ordered.
		if _, err := c.locks.Lock(ctx, s, *own.contactID); err != nil {
			if ent.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("lock contact: %w", err)
		}
	}
	return c.bind(ctx, s, bot, now, own, r)
}

// owner is whose a Binding is: exactly one of a Contact and a Visitor.
type owner struct{ contactID, visitorID *int64 }

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

// bind gives the chat of r to own: a new Binding, or the chat's Binding on bot moved to it. A
// Binding another redemption wrote since the read is found on the second read, which the insert's
// conflict and the read share the key of.
func (c *Chats) bind(ctx context.Context, s *ent.Scoped, bot *ent.Integration, now time.Time, own owner, r Redemption) error {
	for range 2 {
		existing, err := s.Binding().Query().
			Where(binding.IntegrationID(bot.ID), binding.Destination(r.Destination)).
			Only(ctx)
		switch {
		case ent.IsNotFound(err):
			created, err := createBinding(ctx, s, bot, now, own, r)
			switch {
			case err != nil:
				return err
			case created:
				return c.announce(ctx, s, bot, systemevent.TelegramBound, own.contactID, r.DedupKey+"/bound")
			}
			// Another redemption bound the chat since the read: read its Binding.
			continue
		case err != nil:
			return fmt.Errorf("find binding: %w", err)
		}
		return c.rebind(ctx, s, bot, now, own, r, existing)
	}
	return fmt.Errorf("bind: the Binding of the chat on bot %d was neither written nor found", bot.ID)
}

// rebind moves the chat's existing Binding on bot to own, and announces what changed hands.
func (c *Chats) rebind(ctx context.Context, s *ent.Scoped, bot *ent.Integration, now time.Time, own owner, r Redemption, existing *ent.Binding) error {
	if lo.FromPtr(existing.ContactID) == lo.FromPtr(own.contactID) && lo.FromPtr(existing.VisitorID) == lo.FromPtr(own.visitorID) {
		// The chat stays with its owner: nobody gains or loses it, so nothing is announced.
		if lo.FromPtr(existing.ChatID) == r.ChatID {
			return nil
		}
		if err := s.Binding().UpdateOneID(existing.ID).SetChatID(r.ChatID).Exec(ctx); err != nil {
			return fmt.Errorf("update binding chat: %w", err)
		}
		return nil
	}
	move := s.Binding().UpdateOneID(existing.ID).SetChatID(r.ChatID).SetBoundAt(now)
	if own.contactID != nil {
		move.SetContactID(*own.contactID).ClearVisitorID()
	} else {
		move.SetVisitorID(*own.visitorID).ClearContactID()
	}
	if err := move.Exec(ctx); err != nil {
		return fmt.Errorf("move binding: %w", err)
	}
	if err := c.announce(ctx, s, bot, systemevent.TelegramUnbound, existing.ContactID, r.DedupKey+"/unbound"); err != nil {
		return err
	}
	return c.announce(ctx, s, bot, systemevent.TelegramBound, own.contactID, r.DedupKey+"/bound")
}

// createBinding writes the Binding of the chat; false, with nothing written, when the chat already
// has a Binding on bot.
func createBinding(ctx context.Context, s *ent.Scoped, bot *ent.Integration, now time.Time, own owner, r Redemption) (bool, error) {
	c := s.Binding().Create().
		SetIntegrationID(bot.ID).
		SetChannel(binding.ChannelTelegram).
		SetDestination(r.Destination).
		SetChatID(r.ChatID).
		SetBoundAt(now).
		SetNillableContactID(own.contactID).
		SetNillableVisitorID(own.visitorID)
	_, err := c.OnConflictColumns(binding.FieldWorkspaceID, binding.FieldIntegrationID, binding.FieldDestination).DoNothing().ID(ctx)
	created, err := db.Inserted(err)
	if err != nil {
		return false, fmt.Errorf("create binding: %w", err)
	}
	return created, nil
}

// DestinationsOf are the Telegram user ids Bound to the Contact on any bot of the Workspace, its own
// or through one of its Visitors; the Destinations of its Bindings on another Channel are not
// Telegram users.
func (c *Chats) DestinationsOf(ctx context.Context, s *ent.Scoped, contactID int64) ([]string, error) {
	ids, err := s.Binding().Query().
		Where(
			binding.ChannelEQ(binding.ChannelTelegram),
			binding.Or(binding.ContactID(contactID), binding.HasVisitorWith(visitor.ContactID(contactID))),
		).
		Unique(true).
		Select(binding.FieldDestination).
		Strings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list the contact's bindings: %w", err)
	}
	return ids, nil
}

// Removal is a chat Remove unbinds: the Destination on the bot, and the key its event is
// deduplicated by.
type Removal struct {
	Destination string
	DedupKey    string
}

// Remove deletes the Binding of the Destination on bot, a chat Telegram says no longer reaches a
// person (a deactivated user, a chat that does not exist), and records telegram.unbound for the
// Contact it reached, if it was a Contact's. No Binding is nothing to remove. The removal's
// DedupKey keys the event, so a retried removal records it once. Run it inside a transaction of
// the bus that s is bound to.
func (c *Chats) Remove(ctx context.Context, s *ent.Scoped, bot *ent.Integration, r Removal) error {
	b, err := s.Binding().Query().
		Where(binding.IntegrationID(bot.ID), binding.Destination(r.Destination)).
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
	return c.announce(ctx, s, bot, systemevent.TelegramUnbound, b.ContactID, r.DedupKey)
}

// announce records the System event of a Contact gaining or losing the chat; a Visitor with no
// Contact has none to record it for.
func (c *Chats) announce(ctx context.Context, s *ent.Scoped, bot *ent.Integration, action string, contactID *int64, dedup string) error {
	if contactID == nil {
		return nil
	}
	person, err := s.Contact().Get(ctx, *contactID)
	if err != nil {
		return fmt.Errorf("read contact: %w", err)
	}
	return publishChange(ctx, c.bus, s, action, person, bot.ID, dedup)
}

// publishChange records on bus the System event action of person gaining or losing a Binding on
// the Integration integrationID, deduplicated by dedup: the one shape of a BindingChange, whichever
// role moved the Binding.
func publishChange(ctx context.Context, bus *events.Bus, s *ent.Scoped, action string, person *ent.Contact, integrationID int64, dedup string) error {
	return bus.PublishIn(ctx, s, &events.BindingChange{
		Action: action, WorkspaceID: s.WorkspaceID(), ContactID: person.ID, Email: lo.FromPtr(person.Email),
		IntegrationID: integrationID, DedupID: dedup,
	})
}

// Message is a message read from a Destination, whatever it says.
type Message struct {
	Destination string
	Text        string
	// DedupKey is the update's key (callbackinterpretation.Tx.DedupKey).
	DedupKey string
}

// Receive records telegram.received for the Contact the message's Destination is Bound to on bot,
// with the text cut by events.ReceivedText. A Destination that is unknown, or Bound to a Visitor,
// records nothing and creates no Contact. Run it inside a transaction of the bus that s is bound
// to.
func (c *Chats) Receive(ctx context.Context, s *ent.Scoped, bot *ent.Integration, m Message) error {
	b, err := s.Binding().Query().
		Where(binding.IntegrationID(bot.ID), binding.Destination(m.Destination)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find binding: %w", err)
	}
	if b.ContactID == nil {
		return nil
	}
	person, err := s.Contact().Get(ctx, *b.ContactID)
	if err != nil {
		return fmt.Errorf("read contact: %w", err)
	}
	return c.bus.PublishIn(ctx, s, &events.TelegramReceived{
		WorkspaceID: s.WorkspaceID(), ContactID: person.ID, Email: lo.FromPtr(person.Email), IntegrationID: bot.ID,
		Text: events.ReceivedText(m.Text), DedupID: m.DedupKey,
	})
}

// PruneStartTokens deletes the Start tokens that lapsed by the clock or were used, in every
// Workspace, and returns how many went. A lapsed or used token redeems nothing; the tracker and a
// merge tag issue a fresh one per call, so they would otherwise pile up. The one operation of the
// package that takes the raw client, called by a river job: the tokens belong to no one Workspace's
// request.
func (c *Chats) PruneStartTokens(ctx context.Context, client *ent.Client) (int, error) {
	n, err := client.StartToken.Delete().
		Where(starttoken.Or(starttoken.ExpiresAtLTE(c.now()), starttoken.UsedAtNotNil())).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("prune start tokens: %w", err)
	}
	return n, nil
}
