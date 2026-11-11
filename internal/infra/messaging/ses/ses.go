// Package ses implements the Amazon SES email provider. It builds and signs the
// message with the shared messaging.BuildSignedMessage (the same bytes the smtp
// provider sends) and hands them to the SES v2 SendEmail (Content.Raw), so the
// custom headers (List-Unsubscribe, DKIM-Signature) survive intact — the structured
// Simple content would strip them.
//
// The client is built directly on aws-sdk-go-v2's sesv2 client with an optional
// BaseEndpoint override so the same provider can target AWS SES or an
// SES-compatible service such as Yandex Cloud Postbox.
package ses

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/aws/smithy-go"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// Config is the cleartext credential shape stored (encrypted) for an SES
// integration. SecretAccessKey is the only secret; it is never echoed back.
type Config struct {
	Region          string `json:"region"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	From            string `json:"from"`
	FromName        string `json:"fromName,omitempty"`
	// ReplyTo is the default Reply-To of a message that names none; empty sets no header.
	ReplyTo string `json:"replyTo,omitempty"`
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
		Provider: providers.SES,
		Validate: validate,
		Build: func(raw []byte, deps messaging.Deps) (any, error) {
			return build(raw, deps.DKIMKeys, client)
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
		return errors.New("ses: region is required")
	case cfg.AccessKeyID == "":
		return errors.New("ses: accessKeyId is required")
	case cfg.From == "":
		return errors.New("ses: from is required")
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
		return errors.New("ses: endpoint must be an https URL")
	}
	return nil
}

func build(raw []byte, keys messaging.DKIMKeys, client aws.HTTPClient) (any, error) {
	cfg, err := parse(raw)
	if err != nil {
		return nil, err
	}
	return &sender{cfg: cfg, keys: keys, http: client}, nil
}

func parse(raw []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("ses: invalid config: %w", err)
	}
	return cfg, nil
}

type sender struct {
	cfg  Config
	keys messaging.DKIMKeys
	http aws.HTTPClient
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

	client := s.client()
	out, err := client.SendEmail(ctx, &sesv2.SendEmailInput{
		// FromEmailAddress is the envelope sender; the MIME From header carries the
		// display name. It must match a verified SES identity.
		FromEmailAddress: aws.String(msg.From),
		Destination:      &types.Destination{ToAddresses: []string{msg.To}},
		Content:          &types.EmailContent{Raw: &types.RawMessage{Data: wire.Bytes}},
	})
	if err != nil {
		// Only the daily quota needs wording: SES uses the same code for it and
		// the per-second rate. Other capacity replies are identified by code.
		quota := messaging.ClassifyReply(err.Error())
		if errors.Is(quota, messaging.ErrQuotaExceeded) {
			return messaging.Receipt{}, fmt.Errorf("ses: send: %w: %w", quota, err)
		}
		throttled := throttles.IsErrorThrottle(err)
		if throttled == aws.TrueTernary {
			return messaging.Receipt{}, fmt.Errorf("ses: send: %w: %w", messaging.ErrBusy, err)
		}
		if refused(err) {
			return messaging.Receipt{}, fmt.Errorf("ses: send: %w: %w", messaging.ErrRefused, err)
		}
		return messaging.Receipt{}, fmt.Errorf("ses: send: %w", err)
	}
	return messaging.Receipt{MessageID: aws.ToString(out.MessageId)}, nil
}

// refusedCodes are the error codes SES answers with when no retry changes the answer: the
// credentials may not send as the identity (IAM's AccessDenied), the message or its Mail-from
// domain is refused, the identity does not exist. An account that is paused or suspended is not
// among them: a person lifts that, often within the time a job queue keeps retrying.
var refusedCodes = map[string]bool{
	"AccessDeniedException":              true,
	"MessageRejected":                    true,
	"MailFromDomainNotVerifiedException": true,
	"NotFoundException":                  true,
}

// refused tells whether err is one of refusedCodes.
func refused(err error) bool {
	var api smithy.APIError
	return errors.As(err, &api) && refusedCodes[api.ErrorCode()]
}

// SendQuota reads the account's send quota and standing (GetAccount): MaxSendRate messages
// per second and Max24HourSend per rolling 24 hours, EnforcementStatus and production access.
// The API reports whole numbers as floats; a rate below one rounds up to one message per
// second, and a non-positive value (SES reports -1 for an unbounded account) is no ceiling.
//
// The sandbox is AWS's own: an SES-compatible service (an Endpoint of its own) is never in it,
// as an answer that leaves ProductionAccessEnabled out reads as false.
func (s *sender) SendQuota(ctx context.Context) (messaging.Quota, error) {
	client := s.client()
	out, err := client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return messaging.Quota{}, fmt.Errorf("ses: get send quota: %w", err)
	}
	quota := messaging.Quota{
		Enforcement: enforcement(aws.ToString(out.EnforcementStatus)),
		Sandbox:     s.cfg.Endpoint == "" && !out.ProductionAccessEnabled,
	}
	if out.SendQuota != nil {
		quota.PerSecond = quotaValue(out.SendQuota.MaxSendRate)
		quota.PerDay = quotaValue(out.SendQuota.Max24HourSend)
	}
	return quota, nil
}

// enforcement maps SES's EnforcementStatus; HEALTHY, a missing status and one SES may add
// later are no enforcement, so an unknown answer never stops sending.
func enforcement(status string) providers.Enforcement {
	switch status {
	case "PROBATION":
		return providers.EnforcementProbation
	case "SHUTDOWN":
		return providers.EnforcementShutdown
	default:
		return providers.EnforcementNone
	}
}

func quotaValue(v float64) *int {
	if v <= 0 {
		return nil
	}
	n := max(1, int(v))
	return &n
}

// throttles is every error the SDK counts as throttling, its error codes and an unmodeled 429
// alike: the one definition both of what is never retried and of what is Busy.
var throttles = retry.IsErrorThrottles(retry.DefaultThrottles)

// noRetryOnThrottle refuses a retry of any throttle, ahead of the SDK's own retryables, which
// retry some of them each in its own way, and leaves every other error to them.
func noRetryOnThrottle(err error) aws.Ternary {
	throttled := throttles.IsErrorThrottle(err)
	if throttled == aws.TrueTernary {
		return aws.FalseTernary
	}
	return aws.UnknownTernary
}

func (s *sender) client() *sesv2.Client {
	retryer := retry.NewStandard(func(o *retry.StandardOptions) {
		// Transient transport and server failures stay with the SDK; capacity
		// errors reach our Deferral path immediately (ADR 0023).
		o.Retryables = slices.Insert(o.Retryables, 0, retry.IsErrorRetryable(retry.IsErrorRetryableFunc(noRetryOnThrottle)))
	})
	opts := sesv2.Options{
		Region:      s.cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(s.cfg.AccessKeyID, s.cfg.SecretAccessKey, ""),
		HTTPClient:  s.http,
		Retryer:     retryer,
	}
	if s.cfg.Endpoint != "" {
		opts.BaseEndpoint = aws.String(s.cfg.Endpoint)
	}
	return sesv2.New(opts)
}
