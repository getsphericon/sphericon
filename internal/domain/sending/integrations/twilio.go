package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/twilio"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// A Twilio Integration is a Workspace's own Twilio account on the sms Channel (spec #1032,
// "Connecting Twilio"). Its config holds the send credential and the sender; the Auth Token Twilio
// signs its callbacks with is the twilio Preset's auth_token param, so that either is rotated
// alone. Saving it reads the sender with the credentials before the save commits (a wrong or
// revoked key refuses the save) and points the sender's callbacks at the Integration's hook.

// twilioTimeout bounds the Twilio calls of one write.
const twilioTimeout = 30 * time.Second

// TwilioConfig is the write side of a Twilio Integration. A blank Secret on an update keeps the
// stored one while the Account SID and the API key stay the same.
type TwilioConfig struct {
	AccountSID string
	APIKeySID  string
	Secret     string
	From       string
}

// RedactedTwilio is the read side: the account, the key, the sender and whether its callbacks were
// pointed at the hook. Never the secret.
type RedactedTwilio struct {
	AccountSID   string
	APIKeySID    string
	From         string
	FromKind     messaging.SMSFromKind
	CallbacksSet bool
}

// twilioStored is the config a Twilio input names, trimmed.
func twilioStored(c *TwilioConfig) twilio.Config {
	return twilio.Config{
		AccountSID: strings.TrimSpace(c.AccountSID),
		APIKeySID:  strings.TrimSpace(c.APIKeySID),
		Secret:     strings.TrimSpace(c.Secret),
		From:       strings.TrimSpace(c.From),
	}
}

// validTwilio refuses a config of the wrong shape on the field it is about.
func validTwilio(c twilio.Config) error {
	err := twilio.Validate(c)
	if fe, ok := errors.AsType[*twilio.FieldError](err); ok {
		return refuse(fieldConfig+"."+fe.Field, twilioFieldMessage(fe), nil)
	}
	return err
}

func twilioFieldMessage(fe *twilio.FieldError) i18n.ID {
	switch {
	case errors.Is(fe, twilio.ErrAccountSID):
		return i18n.ErrorsIntegrationTwilioAccountSid
	case errors.Is(fe, twilio.ErrAPIKeySID):
		return i18n.ErrorsIntegrationTwilioApiKeySid
	case errors.Is(fe, twilio.ErrSecret):
		return i18n.ErrorsIntegrationTwilioSecret
	}
	return i18n.ErrorsIntegrationTwilioFrom
}

// twilioConfigOf opens a Twilio Integration's stored config.
func (m *Module) twilioConfigOf(row *ent.Integration) (twilio.Config, error) {
	plaintext, err := m.cipher.Decrypt(row.ConfigEncrypted)
	if err != nil {
		return twilio.Config{}, err
	}
	var c twilio.Config
	err = json.Unmarshal(plaintext, &c)
	return c, err
}

// connectTwilio reads the sender of c with its credentials and points its callbacks at the hook of
// key. It returns the config to store, which records whether the callbacks were set.
func (m *Module) connectTwilio(ctx context.Context, c twilio.Config, key string) ([]byte, error) {
	if err := validTwilio(c); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	account, err := m.catalog.BuildSMS(providers.Twilio, raw)
	if err != nil {
		return nil, configError(err)
	}
	ctx, cancel := context.WithTimeout(ctx, twilioTimeout)
	defer cancel()
	from, err := account.Sender(ctx)
	if err != nil {
		return nil, twilioRefusal(err, c.From)
	}
	if c.CallbacksSet, err = account.PointCallbacks(ctx, from, m.hookURL(key)); err != nil {
		return nil, twilioRefusal(err, c.From)
	}
	return json.Marshal(c)
}

// twilioRefusal turns a Twilio failure into a refusal of the write: credentials Twilio does not
// accept on the secret, a sender the account does not have on from, any other answer of Twilio in
// its own words, and an unreachable Twilio as such.
func twilioRefusal(err error, from string) error {
	switch {
	case errors.Is(err, messaging.ErrSMSUnauthorized):
		return refuse(fieldConfig+".secret", i18n.ErrorsIntegrationTwilioRefused, nil)
	case errors.Is(err, messaging.ErrSMSSenderNotFound):
		return refuse(fieldConfig+".from", i18n.ErrorsIntegrationTwilioSenderNotFound, map[string]any{"From": from})
	}
	if e, ok := errors.AsType[*messaging.SMSError](err); ok {
		return refuse(fieldConfig, i18n.ErrorsIntegrationTwilioError, map[string]any{detailKey: e.Message})
	}
	return refuse(fieldConfig, i18n.ErrorsIntegrationTwilioUnreachable, map[string]any{detailKey: err.Error()})
}

// editTwilio validates an Update of a Twilio Integration and, when it changes the credentials or the
// sender, connects them before the save commits. A blank secret keeps the stored one only while the
// Account SID and the API key stay the same. It returns the sealed config to store, nil when the
// edit leaves it.
func (m *Module) editTwilio(ctx context.Context, row *ent.Integration, in UpdateInput) (*string, error) {
	if name := in.Preset.Over(row.Preset); in.Preset.Changes() && (name == nil || *name != preset.BuiltinTwilio) {
		return nil, refuse(fieldPreset, i18n.ErrorsIntegrationTwilioPresetFixed, nil)
	}
	cfg, ok := in.Config.Get()
	if !ok {
		return nil, nil
	}
	if cfg.Twilio == nil {
		return nil, refuse(fieldConfig, i18n.ErrorsIntegrationKindMismatch, nil)
	}
	stored, err := m.twilioConfigOf(row)
	if err != nil {
		return nil, err
	}
	next := twilioStored(cfg.Twilio)
	if next.Secret == "" {
		if next.AccountSID != stored.AccountSID || next.APIKeySID != stored.APIKeySID {
			return nil, refuse(fieldConfig+".secret", i18n.ErrorsIntegrationTwilioSecretRequired, nil)
		}
		next.Secret = stored.Secret
	}
	next.CallbacksSet = stored.CallbacksSet
	// The same credentials saved again change nothing, unless Twilio refused them at a send: the
	// Owner saves them once the account works again, and they are checked anew and the hold goes.
	if next == stored && row.ProviderRefusedAt == nil {
		return nil, nil
	}
	plaintext, err := m.connectTwilio(ctx, next, row.IngestKey)
	if err != nil {
		return nil, err
	}
	enc, err := m.cipher.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	return &enc, nil
}

// repointTwilio points a stored Twilio Integration's sender at the hook of key, as a new key asks,
// and returns the sealed config to store.
func (m *Module) repointTwilio(ctx context.Context, row *ent.Integration, key string) (string, error) {
	stored, err := m.twilioConfigOf(row)
	if err != nil {
		return "", err
	}
	plaintext, err := m.connectTwilio(ctx, stored, key)
	if err != nil {
		return "", err
	}
	return m.cipher.Encrypt(plaintext)
}

// redactTwilio is the read side of a stored Twilio config.
func redactTwilio(plaintext []byte) (RedactedConfig, error) {
	var c twilio.Config
	if err := json.Unmarshal(plaintext, &c); err != nil {
		return RedactedConfig{}, err
	}
	kind, _ := twilio.FromKind(c.From)
	return RedactedConfig{Twilio: &RedactedTwilio{
		AccountSID: c.AccountSID, APIKeySID: c.APIKeySID, From: c.From, FromKind: kind, CallbacksSet: c.CallbacksSet,
	}}, nil
}
