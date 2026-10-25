package messaging

import (
	"context"
	"fmt"
)

// ProviderDescriptor is the extensibility seam: it teaches the catalog how to
// validate and build one provider. Build returns a channel-specific sender as
// `any`; callers type-assert to the channel interface (e.g. EmailSender).
type ProviderDescriptor struct {
	Channel  Channel
	Provider Provider

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
	// Signer DKIM-signs outbound mail per ADR 0010; may be nil.
	Signer Signer
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

// Catalog holds the registered provider descriptors keyed by (channel, provider).
type Catalog struct {
	descriptors map[catalogKey]ProviderDescriptor
}

type catalogKey struct {
	channel  Channel
	provider Provider
}

// NewCatalog builds a catalog from the given descriptors.
func NewCatalog(descriptors ...ProviderDescriptor) *Catalog {
	c := &Catalog{descriptors: make(map[catalogKey]ProviderDescriptor, len(descriptors))}
	for _, d := range descriptors {
		c.descriptors[catalogKey{d.Channel, d.Provider}] = d
	}
	return c
}

// Descriptor looks up a registered provider; ok is false for unknown pairs.
func (c *Catalog) Descriptor(channel Channel, provider Provider) (ProviderDescriptor, bool) {
	d, ok := c.descriptors[catalogKey{channel, provider}]
	return d, ok
}

// ChannelOf reports which channel a provider belongs to; ok is false for an
// unregistered provider.
func (c *Catalog) ChannelOf(provider Provider) (Channel, bool) {
	for k := range c.descriptors {
		if k.provider == provider {
			return k.channel, true
		}
	}
	return "", false
}

// Validate runs the provider's semantic validation, or reports an unknown provider.
func (c *Catalog) Validate(channel Channel, provider Provider, config []byte) error {
	d, ok := c.Descriptor(channel, provider)
	if !ok {
		return fmt.Errorf("unknown provider %q for channel %q", provider, channel)
	}
	if d.Validate == nil {
		return nil
	}
	return d.Validate(config)
}

// BuildEmail builds an EmailSender for an email-channel provider from deps.
func (c *Catalog) BuildEmail(provider Provider, config []byte, deps Deps) (EmailSender, error) {
	d, ok := c.Descriptor(ChannelEmail, provider)
	if !ok {
		return nil, fmt.Errorf("unknown email provider %q", provider)
	}
	built, err := d.Build(config, deps)
	if err != nil {
		return nil, err
	}
	sender, ok := built.(EmailSender)
	if !ok {
		return nil, fmt.Errorf("provider %q did not build an EmailSender", provider)
	}
	return instrumentedSender{EmailSender: sender, provider: provider}, nil
}
