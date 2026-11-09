// Package smtp implements the SMTP email provider on top of wneessen/go-mail.
// It sends the bytes of messaging.BuildSignedMessage, as the SES provider does, so
// the wire message is identical across providers; only the transport differs.
package smtp

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/textproto"
	"slices"

	"github.com/wneessen/go-mail"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
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
	// ReplyTo is the default Reply-To of a message that names none; empty sets no header.
	ReplyTo string `json:"replyTo,omitempty"`
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
		Channel:  channels.Email,
		Provider: providers.SMTP,
		Validate: func(raw []byte) error { return validate(raw, allowPrivate) },
		Build: func(raw []byte, deps messaging.Deps) (any, error) {
			return build(raw, deps.DKIMKeys, allowPrivate)
		},
	}
}

var mailPorts = []uint16{25, 465, 587, 2525}

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
	case !allowPrivate && !slices.ContainsFunc(mailPorts, func(p uint16) bool { return int(p) == cfg.Port }):
		return errors.New("smtp: port must be one of 25, 465, 587, 2525")
	}
	return nil
}

func build(raw []byte, keys messaging.DKIMKeys, allowPrivate bool) (any, error) {
	cfg, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return &sender{cfg: cfg, keys: keys, allowPrivate: allowPrivate}, nil
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
	keys         messaging.DKIMKeys
	allowPrivate bool
}

// DefaultFrom reports the integration's configured From (see messaging.DefaultFromer).
func (s *sender) DefaultFrom() (address, name string) { return s.cfg.From, s.cfg.FromName }

func (s *sender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	msg.From = cmp.Or(msg.From, s.cfg.From)
	msg.FromName = cmp.Or(msg.FromName, s.cfg.FromName)
	msg.ReplyTo = cmp.Or(msg.ReplyTo, s.cfg.ReplyTo)

	wire, err := messaging.BuildSignedMessage(ctx, msg, s.keys)
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
	if err := deliver(ctx, client, wire); err != nil {
		// Only a transient (4xx) reply can mean "too fast"; a permanent one never does.
		reply, isReply := errors.AsType[*textproto.Error](err)
		transient := isReply && reply.Code/100 == 4
		if transient {
			if busy := messaging.ClassifyReply(err.Error()); busy != nil {
				return messaging.Receipt{}, fmt.Errorf("%w: %w", busy, err)
			}
		}
		return messaging.Receipt{}, err
	}
	return messaging.Receipt{MessageID: wire.MessageID}, nil
}

// deliver connects as client is configured to (dialer, TLS policy, auth) and sends the bytes of
// wire, signed as they are, in one mail transaction. Ending the session (QUIT) is a cleanup step:
// the server has answered for the message by then, and failing the send over it would send the
// message again.
func deliver(ctx context.Context, client *mail.Client, wire messaging.Wire) error {
	conn, err := client.DialToSMTPClientWithContext(ctx)
	if err != nil {
		return fmt.Errorf("smtp: dial: %w", err)
	}
	defer func() {
		quitErr := client.CloseWithSMTPClient(conn)
		cleanup.Report("end the smtp session", quitErr)
	}()

	if err := conn.Mail(wire.Sender); err != nil {
		return fmt.Errorf("smtp: mail from: %w", err)
	}
	for _, rcpt := range wire.Recipients {
		if err := conn.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp: rcpt to: %w", err)
		}
	}
	data, err := conn.Data()
	if err != nil {
		return fmt.Errorf("smtp: data: %w", err)
	}
	if _, err := data.Write(wire.Bytes); err != nil {
		return fmt.Errorf("smtp: write message: %w", err)
	}
	// The server answers the message itself when its data ends: an accepted or refused message
	// is known only here.
	if err := data.Close(); err != nil {
		return fmt.Errorf("smtp: end data: %w", err)
	}
	return nil
}
