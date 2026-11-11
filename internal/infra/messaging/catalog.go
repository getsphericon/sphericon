package messaging

import (
	"context"
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// ProviderDescriptor is the extensibility seam: it teaches the catalog how to
// validate and build one provider. Build returns a sender of the provider's Channel
// (providers.Provider.Channel) as `any`; callers type-assert to the channel interface
// (e.g. EmailSender).
type ProviderDescriptor struct {
	Provider providers.Provider

	// Validate runs semantic checks on a decrypted, structurally-valid config
	// blob (the typed API union already enforces shape and required fields).
	Validate func(config []byte) error

	// Build constructs the live sender from a decrypted config blob and the
	// dependencies bound to the Workspace it sends for. A provider uses what it needs
	// and ignores the rest.
	Build func(config []byte, deps Deps) (any, error)
}

// Deps is what the catalog hands every provider builder, bound to one Workspace's
// scoped handle (zero fields when a sender is built outside a send, e.g. for a quota
// lookup).
type Deps struct {
	// DKIMKeys finds the key outbound mail is DKIM-signed with (ADR 0010); nil for
	// mail that goes out unsigned (system mail).
	DKIMKeys DKIMKeys
	// Capturer keeps the messages of a provider that delivers nowhere outside (the
	// Test inbox); may be nil, and a provider that delivers outside ignores it.
	Capturer Capturer
}

// Capturer keeps a message a provider accepted instead of sending it out. It is bound to
// one Workspace and one Integration.
type Capturer interface {
	Capture(ctx context.Context, c Captured) error
}

// Captured is a message as a provider that delivers nowhere outside keeps it: the message,
// the Message-ID it gave it, the header lines it would carry on the wire in order, and the
// URLs of the links of its HTML part.
type Captured struct {
	Message   EmailMessage
	MessageID string
	Headers   []Header
	Links     []string
}

// Header is one header line of a message.
type Header struct {
	Name  string
	Value string
}

// Catalog holds the registered provider descriptors keyed by provider.
type Catalog struct {
	descriptors map[providers.Provider]ProviderDescriptor
}

// NewCatalog builds a catalog from the given descriptors.
func NewCatalog(descriptors ...ProviderDescriptor) *Catalog {
	c := &Catalog{descriptors: make(map[providers.Provider]ProviderDescriptor, len(descriptors))}
	for _, d := range descriptors {
		c.descriptors[d.Provider] = d
	}
	return c
}

// Descriptor looks up a registered provider; ok is false for an unknown one.
func (c *Catalog) Descriptor(provider providers.Provider) (ProviderDescriptor, bool) {
	d, ok := c.descriptors[provider]
	return d, ok
}

// Validate runs the provider's semantic validation, or reports an unknown provider.
func (c *Catalog) Validate(provider providers.Provider, config []byte) error {
	d, ok := c.Descriptor(provider)
	if !ok {
		return fmt.Errorf("unknown provider %q", provider)
	}
	if d.Validate == nil {
		return nil
	}
	return d.Validate(config)
}

// Build builds the sender of a provider from deps: a value of its Channel's own send
// interface (EmailSender on email), decorated as every sender of the Channel is. Adding a Channel
// adds its descriptors and its entry in instruments.
func (c *Catalog) Build(provider providers.Provider, config []byte, deps Deps) (any, error) {
	d, ok := c.Descriptor(provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
	channel := provider.Channel()
	instrument, ok := instruments[channel]
	if !ok {
		return nil, fmt.Errorf("no sender of channel %q is built", channel)
	}
	built, err := d.Build(config, deps)
	if err != nil {
		return nil, err
	}
	return instrument(provider, built)
}

// instruments checks that a freshly built sender of each Channel is of the Channel's send
// interface and decorates it: with the metrics decorator on email.
var instruments = map[channels.Channel]func(providers.Provider, any) (any, error){channels.Email: func(provider providers.Provider, built any) (any, error) {
	sender, ok := built.(EmailSender)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build an EmailSender", provider)
	}
	return instrumentedSender{EmailSender: sender, provider: provider}, nil
}, channels.Telegram: func(provider providers.Provider, built any) (any, error) {
	bot, ok := built.(TelegramBot)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build a TelegramBot", provider)
	}
	return bot, nil
}, channels.SMS: func(provider providers.Provider, built any) (any, error) {
	sender, ok := built.(SMSSender)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build an SMSSender", provider)
	}
	return sender, nil
},
}

// BuildEmail builds an EmailSender for an email-channel provider from deps: Build, typed.
func (c *Catalog) BuildEmail(provider providers.Provider, config []byte, deps Deps) (EmailSender, error) {
	built, err := c.Build(provider, config, deps)
	if err != nil {
		return nil, err
	}
	sender, ok := built.(EmailSender)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build an EmailSender", provider)
	}
	return sender, nil
}

// BuildTelegram builds the TelegramBot of a telegram-channel provider: Build, typed.
func (c *Catalog) BuildTelegram(provider providers.Provider, config []byte) (TelegramBot, error) {
	built, err := c.Build(provider, config, Deps{})
	if err != nil {
		return nil, err
	}
	bot, ok := built.(TelegramBot)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build a TelegramBot", provider)
	}
	return bot, nil
}

// BuildSMS builds the SMSSender of an sms-channel provider: Build, typed.
func (c *Catalog) BuildSMS(provider providers.Provider, config []byte) (SMSSender, error) {
	built, err := c.Build(provider, config, Deps{})
	if err != nil {
		return nil, err
	}
	sender, ok := built.(SMSSender)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build an SMSSender", provider)
	}
	return sender, nil
}
