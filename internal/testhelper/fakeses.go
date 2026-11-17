package testhelper

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/inbox"
	"github.com/getsphericon/sphericon/internal/infra/messaging/ses"
	"github.com/getsphericon/sphericon/internal/infra/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// FakeSES stands in for the SES account behind every "ses" Integration the server
// builds in a test: its send quota and whether it has an Integration's configuration set are
// whatever the test scripts (by default none is reported, the set exists and every lookup
// succeeds), so no test ever reaches the real SES API.
type FakeSES struct {
	mu         sync.Mutex
	quota      messaging.Quota
	err        error
	setMissing bool
	setErr     error
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

// SetConfigurationSetMissing makes the next GetConfigurationSet calls answer that the account has
// no such set (true) or has it (false), and succeed.
func (f *FakeSES) SetConfigurationSetMissing(missing bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setMissing, f.setErr = missing, nil
}

// SetConfigurationSetErr makes the next GetConfigurationSet calls fail with err (nil clears it):
// a missing ses:GetConfigurationSet permission, or an SES-compatible service without the call.
func (f *FakeSES) SetConfigurationSetErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setErr = err
}

// ConfigurationSetMissing answers the scripted set lookup.
func (f *FakeSES) ConfigurationSetMissing(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.setMissing, f.setErr
}

// Catalog is a catalog whose "ses" provider builds the fake itself, so discovery
// reaches it through the stored, encrypted Integration config exactly as it reaches
// the real sender; "smtp" builds a sender that cannot report a quota.
func (f *FakeSES) Catalog() *messaging.Catalog {
	return messaging.NewCatalog(
		messaging.ProviderDescriptor{
			Provider: providers.SES,
			Build:    func([]byte, messaging.Deps) (any, error) { return f, nil },
		},
		messaging.ProviderDescriptor{
			Provider: providers.SMTP,
			Build:    func([]byte, messaging.Deps) (any, error) { return plainSender{}, nil },
		},
	)
}

type plainSender struct{}

func (plainSender) Send(context.Context, messaging.EmailMessage) (messaging.Receipt, error) {
	return messaging.Receipt{}, nil
}

// quotaSender is the real SES sender with its quota and configuration set lookups answered by
// the fake.
type quotaSender struct {
	messaging.EmailSender

	fake *FakeSES
}

func (s quotaSender) SendQuota(ctx context.Context) (messaging.Quota, error) {
	return s.fake.SendQuota(ctx)
}

func (s quotaSender) ConfigurationSetMissing(ctx context.Context) (bool, error) {
	return s.fake.ConfigurationSetMissing(ctx)
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
