package integrations

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/ses"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// fieldConfigurationSet is the field a refusal of an SES configuration set's name is on.
const fieldConfigurationSet = fieldConfig + ".configurationSet"

// validConfigurationSet refuses a configuration set name the contract does not allow; an empty one
// names no set.
func validConfigurationSet(set string) error {
	if set == "" {
		return nil
	}
	if err := validation.Length(fieldConfigurationSet, set,
		bounds.SesConfigurationSetMinLength, bounds.SesConfigurationSetMaxLength); err != nil {
		return err
	}
	return validation.Pattern(fieldConfigurationSet, set, bounds.SesConfigurationSetPattern)
}

// checkConfigurationSet asks SES whether the configuration set an SES config about to be saved
// names exists, and returns the config with the answer recorded: ConfigurationSetMissing only
// when SES says it has no such set. The check is best effort, like the quota lookup: an error
// (no ses:GetConfigurationSet permission, an SES-compatible service without the call, a timeout)
// is logged and records nothing, and neither answer refuses the save or holds a send. A config
// that names no set asks nothing. It runs before the save's transaction opens.
func (m *Module) checkConfigurationSet(ctx context.Context, plaintext []byte) ([]byte, error) {
	var c ses.Config
	if err := json.Unmarshal(plaintext, &c); err != nil {
		return nil, err
	}
	c.ConfigurationSetMissing = false
	if c.ConfigurationSet != "" {
		missing, err := m.configurationSetMissing(ctx, plaintext)
		if err != nil {
			slog.WarnContext(ctx, "ses configuration set unchecked", "configuration_set", c.ConfigurationSet, "err", err)
		}
		c.ConfigurationSetMissing = missing
	}
	return json.Marshal(c)
}

// configurationSetMissing builds the SES sender of plaintext and asks it, within quotaTimeout.
func (m *Module) configurationSetMissing(ctx context.Context, plaintext []byte) (bool, error) {
	sender, err := m.catalog.BuildEmail(providers.SES, plaintext, messaging.Deps{})
	if err != nil {
		return false, err
	}
	checker, ok := messaging.AsConfigurationSetChecker(sender)
	if !ok {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	return checker.ConfigurationSetMissing(ctx)
}
