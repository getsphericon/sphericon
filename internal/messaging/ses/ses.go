// Package ses implements the Amazon SES email provider. It builds the message
// with the shared messaging.BuildMIME (same MIME as the smtp provider) and
// hands the raw bytes to SES SendRawEmail, so any custom headers (List-Unsubscribe,
// DKIM later) survive intact — SendEmail's structured API would strip them.
//
// The client is built directly on aws-sdk-go-v2's ses client with an optional
// BaseEndpoint override so the same provider can target AWS SES or an
// SES-compatible service such as Yandex Cloud Postbox.
package ses

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"

	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// Config is the cleartext credential shape stored (encrypted) for an SES
// integration. SecretAccessKey is the only secret; it is never echoed back.
type Config struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	From            string `json:"from"`
	FromName        string `json:"fromName,omitempty"`
	// Endpoint targets an SES-compatible API (e.g. Yandex Cloud Postbox) instead
	// of AWS. Empty ⇒ AWS default endpoint for Region. Not a secret.
	Endpoint string `json:"endpoint,omitempty"`
}

// requestTimeout bounds one SES API call made through the SSRF-hardened client.
const requestTimeout = 30 * time.Second

// Descriptor registers the SES provider with the messaging catalog. Its requests go through
// an SSRF-hardened client (the webhook one): the Endpoint is customer-supplied, so a
// private, loopback or metadata address is refused when dialled, on the resolved IP.
func Descriptor() messaging.ProviderDescriptor {
	return NewDescriptor(webhook.NewClient(requestTimeout))
}

// NewDescriptor is Descriptor with the HTTP client the SES API is reached through, so a
// test can point the provider at a local fake. Production uses Descriptor.
func NewDescriptor(client aws.HTTPClient) messaging.ProviderDescriptor {
	return messaging.ProviderDescriptor{
		Channel:  messaging.ChannelEmail,
		Provider: messaging.ProviderSES,
		Validate: validate,
		Build: func(raw []byte, signer messaging.Signer) (any, error) {
			return build(raw, signer, client)
		},
	}
}

func validate(raw []byte) error {
	cfg, err := parse(raw)
	if err != nil {
		return err
	}
	switch {
	case cfg.Region == "":
		return fmt.Errorf("ses: region is required")
	case cfg.AccessKeyID == "":
		return fmt.Errorf("ses: accessKeyId is required")
	case cfg.From == "":
		return fmt.Errorf("ses: from is required")
	}
	return validEndpoint(cfg.Endpoint)
}

// validEndpoint accepts no endpoint (AWS) or an absolute https URL with a host. Which
// addresses it may reach is decided when the request dials, not here.
func validEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return fmt.Errorf("ses: endpoint must be an https URL")
	}
	return nil
}

func build(raw []byte, signer messaging.Signer, client aws.HTTPClient) (any, error) {
	cfg, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return &sender{cfg: cfg, signer: signer, http: client}, nil
}

func parse(raw []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("ses: invalid config: %w", err)
	}
	return cfg, nil
}

type sender struct {
	cfg    Config
	signer messaging.Signer
	http   aws.HTTPClient
}

// DefaultFrom reports the integration's configured From (see messaging.DefaultFromer).
func (s *sender) DefaultFrom() (string, string) { return s.cfg.From, s.cfg.FromName }

func (s *sender) Send(ctx context.Context, msg messaging.EmailMessage) (messaging.Receipt, error) {
	msg.From = messaging.FirstNonEmpty(msg.From, s.cfg.From)
	msg.FromName = messaging.FirstNonEmpty(msg.FromName, s.cfg.FromName)

	m, err := messaging.BuildSignedMIME(ctx, msg, s.signer)
	if err != nil {
		return messaging.Receipt{}, err
	}
	var raw bytes.Buffer
	if _, err := m.WriteTo(&raw); err != nil {
		return messaging.Receipt{}, fmt.Errorf("ses: render message: %w", err)
	}

	client, err := s.client(ctx)
	if err != nil {
		return messaging.Receipt{}, err
	}
	out, err := client.SendRawEmail(ctx, &ses.SendRawEmailInput{
		// Source is the envelope sender; the MIME From header carries the display
		// name. It must match a verified SES identity.
		Source:       aws.String(msg.From),
		Destinations: []string{msg.To},
		RawMessage:   &types.RawMessage{Data: raw.Bytes()},
	})
	if err != nil {
		if busy := messaging.ClassifyReply(err.Error()); busy != nil {
			// SES Throttling (rate or daily quota): the provider is busy, not the
			// message bad (ADR 0023).
			return messaging.Receipt{}, fmt.Errorf("ses: send: %w: %w", busy, err)
		}
		return messaging.Receipt{}, fmt.Errorf("ses: send: %w", err)
	}
	return messaging.Receipt{MessageID: aws.ToString(out.MessageId)}, nil
}

// SendQuota reads the account's send quota (GetSendQuota): MaxSendRate messages per
// second and Max24HourSend per rolling 24 hours. The API reports whole numbers as
// floats; a rate below one rounds up to one message per second, and a non-positive
// value (SES reports -1 for an unbounded account) is no ceiling.
func (s *sender) SendQuota(ctx context.Context) (messaging.Quota, error) {
	client, err := s.client(ctx)
	if err != nil {
		return messaging.Quota{}, err
	}
	out, err := client.GetSendQuota(ctx, &ses.GetSendQuotaInput{})
	if err != nil {
		return messaging.Quota{}, fmt.Errorf("ses: get send quota: %w", err)
	}
	return messaging.Quota{
		PerSecond: quotaValue(out.MaxSendRate),
		PerDay:    quotaValue(out.Max24HourSend),
	}, nil
}

func quotaValue(v float64) *int {
	if v <= 0 {
		return nil
	}
	n := max(1, int(v))
	return &n
}

func (s *sender) client(ctx context.Context) (*ses.Client, error) {
	creds := credentials.NewStaticCredentialsProvider(s.cfg.AccessKeyID, s.cfg.SecretAccessKey, "")
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithCredentialsProvider(creds),
		config.WithRegion(s.cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("ses: load config: %w", err)
	}
	return ses.NewFromConfig(cfg, func(o *ses.Options) {
		o.HTTPClient = s.http
		if s.cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(s.cfg.Endpoint)
		}
	}), nil
}
