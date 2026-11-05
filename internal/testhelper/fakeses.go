package testhelper

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/getsphericon/sphericon/internal/channels"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/inbox"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/messaging/telegram"
	"github.com/getsphericon/sphericon/internal/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/providers"
)

// FakeSES stands in for the SES account behind every "ses" Integration the server
// builds in a test: its send quota is whatever the test scripts (by default none is
// reported and the lookup succeeds), so no test ever reaches the real SES API.
type FakeSES struct {
	mu    sync.Mutex
	quota messaging.Quota
	err   error
}

// SetQuota makes the next GetSendQuota calls report quota (and succeed).
func (f *FakeSES) SetQuota(quota messaging.Quota) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quota, f.err = quota, nil
}

// SetQuotaErr makes the next GetSendQuota calls fail with err (nil clears it): a
// missing ses:GetSendQuota permission, or an SES-compatible service without the call.
func (f *FakeSES) SetQuotaErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Send is never reached: the fake answers quota lookups only.
func (f *FakeSES) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, errors.New("FakeSES does not send")
}

// SendQuota answers the scripted quota or error.
func (f *FakeSES) SendQuota(context.Context) (messaging.Quota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quota, f.err
}

// Catalog is a catalog whose "ses" provider builds the fake itself, so discovery
// reaches it through the stored, encrypted Integration config exactly as it reaches
// the real sender; "smtp" builds a sender that cannot report a quota.
func (f *FakeSES) Catalog() *messaging.Catalog {
	return messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Channel: channels.Email, Provider: providers.SES,
			Build: func([]byte, messaging.Deps) (any, error) { return f, nil },
		},
		messaging.ProviderDescriptor{
			Channel: channels.Email, Provider: providers.SMTP,
			Build: func([]byte, messaging.Deps) (any, error) { return plainSender{}, nil },
		},
	)
}

type plainSender struct{}

func (plainSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, nil
}

// quotaSender is the real SES sender with its quota lookup answered by the fake.
type quotaSender struct {
	messaging.EmailSender

	fake *FakeSES
}

func (s quotaSender) SendQuota(ctx context.Context) (messaging.Quota, error) {
	return s.fake.SendQuota(ctx)
}

// catalogWith is the built-in catalog with SES's quota lookup answered by fake, and Telegram's
// bots reached at telegramAPIURL (the fake Bot API of the env) and Twilio's accounts at twilioAPIURL
// (the fake REST API of the env).
func catalogWith(fake *FakeSES, telegramAPIURL, twilioAPIURL string) *messaging.Catalog {
	descriptor := ses.Descriptor()
	descriptor.Build = func(config []byte, deps messaging.Deps) (any, error) {
		built, err := ses.Descriptor().Build(config, deps)
		if err != nil {
			return nil, err
		}
		sender, ok := built.(messaging.EmailSender)
		if !ok {
			return nil, fmt.Errorf("the ses descriptor built a %T, not a messaging.EmailSender", built)
		}
		return quotaSender{EmailSender: sender, fake: fake}, nil
	}
	return messaging.NewCatalog(smtp.Descriptor(), descriptor, inbox.Descriptor(), telegram.Descriptor(telegramAPIURL), twilio.Descriptor(twilioAPIURL))
}
