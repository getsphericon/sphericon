// Package smtp implements the SMTP email provider on top of wneessen/go-mail.
// It shares messaging.BuildMIME with the SES provider so the wire message is
// identical across providers; only the transport differs.
package smtp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/internal/messaging"
)

// Config is the cleartext credential shape stored (encrypted) for an SMTP
// integration. Password is the only secret; it is never echoed back by the API.
type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from"`
	FromName string `json:"fromName,omitempty"`
}

// Descriptor registers the SMTP provider with the messaging catalog, for tenant-supplied
// config: private hosts and unusual ports are refused (see NewDescriptor).
func Descriptor() messaging.ProviderDescriptor { return NewDescriptor(false) }

// NewDescriptor registers the SMTP provider. Unless allowPrivate, a config may only name a
// standard mail port (25, 465, 587, 2525), and the connection is refused at dial time when
// the host resolves to a loopback, private, link-local or otherwise non-public address, so
// the stored password cannot be sent to an internal service nor the server used to probe one.
func NewDescriptor(allowPrivate bool) messaging.ProviderDescriptor {
	return messaging.ProviderDescriptor{
		Channel:  messaging.ChannelEmail,
		Provider: messaging.ProviderSMTP,
		Validate: func(raw []byte) error { return validate(raw, allowPrivate) },
		Build: func(raw []byte, deps messaging.Deps) (any, error) {
			return build(raw, deps.Signer, allowPrivate)
		},
	}
}

var mailPorts = []int{25, 465, 587, 2525}

func validate(raw []byte, allowPrivate bool) error {
	cfg, err := parse(raw)
	if err != nil {
		return err
	}
	switch {
	case cfg.Host == "":
		return errors.New("smtp: host is required")
	case cfg.Port <= 0:
		return errors.New("smtp: port must be positive")
	case cfg.From == "":
		return errors.New("smtp: from is required")
	case !allowPrivate && !slices.Contains(mailPorts, cfg.Port):
		return errors.New("smtp: port must be one of 25, 465, 587, 2525")
	}
	return nil
}

func build(raw []byte, signer messaging.Signer, allowPrivate bool) (any, error) {
	cfg, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return &sender{cfg: cfg, signer: signer, allowPrivate: allowPrivate}, nil
}

func parse(raw []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("smtp: invalid config: %w", err)
	}
	return cfg, nil
}

type sender struct {
	cfg          Config
	signer       messaging.Signer
	allowPrivate bool
}

// DefaultFrom reports the integration's configured From (see messaging.DefaultFromer).
func (s *sender) DefaultFrom() (address, name string) { return s.cfg.From, s.cfg.FromName }

func (s *sender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	msg.From = cmp.Or(msg.From, s.cfg.From)
	msg.FromName = cmp.Or(msg.FromName, s.cfg.FromName)

	m, err := messaging.BuildSignedMIME(ctx, msg, s.signer)
	if err != nil {
		return messaging.Receipt{}, err
	}

	// Opportunistic STARTTLS: encrypt when the server offers it (prod SMTP),
	// fall back to plaintext for local relays like mailpit that don't advertise
	// it. go-mail defaults to mandatory STARTTLS, which would break dev sends.
	opts := []mail.Option{
		mail.WithPort(s.cfg.Port),
		mail.WithTLSPortPolicy(mail.TLSOpportunistic),
	}
	if !s.allowPrivate {
		opts = append(opts, mail.WithDialContextFunc(publicDialer().DialContext))
	}
	// Authenticate whenever a credential is set; an empty pair means an open relay.
	if s.cfg.Username != "" || s.cfg.Password != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(s.cfg.Username),
			mail.WithPassword(s.cfg.Password),
		)
	}

	client, err := mail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("smtp: client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, m); err != nil {
		// Only a transient (4xx) reply can mean "too fast"; a permanent one never does.
		var sendErr *mail.SendError
		if errors.As(err, &sendErr) && sendErr.IsTemp() {
			if busy := messaging.ClassifyReply(err.Error()); busy != nil {
				return messaging.Receipt{}, fmt.Errorf("%w: %w", busy, err)
			}
		}
		return messaging.Receipt{}, err
	}
	return messaging.Receipt{MessageID: m.GetMessageID()}, nil
}
