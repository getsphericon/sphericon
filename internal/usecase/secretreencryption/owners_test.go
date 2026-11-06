package secretreencryption_test

import (
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/rules"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
	"github.com/getsphericon/sphericon/internal/staffmembers"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/secretreencryption"
	"github.com/getsphericon/sphericon/internal/users"
	"github.com/getsphericon/sphericon/internal/webhooks"
)

// reencryption builds the use case the way the wiring does, on the owners' stored secrets over the
// test's raw client, with cipher as the target.
func reencryption(env *testhelper.TestEnv, cipher *secrets.Cipher) *secretreencryption.UseCase {
	return secretreencryption.New(secretreencryption.Owners{
		Integrations:     integrations.NewSecrets(env.DB),
		WebhookEndpoints: webhooks.NewSecrets(env.DB),
		SendingDomains:   sendingdomains.NewSecrets(env.DB),
		Users:            users.NewSecrets(env.DB),
		StaffMembers:     staffmembers.NewSecrets(env.DB),
		Automations:      automations.NewSecrets(env.DB),
		Rules:            rules.NewSecrets(env.DB),
	}, cipher)
}
