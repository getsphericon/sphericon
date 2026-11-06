package secretreencryption_test

import (
	"github.com/getsphericon/sphericon/internal/domain/identity/staffmembers"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/secretreencryption"
)

// reencryption builds the use case the way the wiring does, on the owners' stored secrets over the
// test's raw client, with cipher as the target.
func reencryption(env *testhelper.TestEnv, cipher *secrets.Cipher) *secretreencryption.UseCase {
	return secretreencryption.New(secretreencryption.Owners{
		Integrations:     integrations.NewSecrets(env.DB),
		WebhookEndpoints: webhookendpoints.NewSecrets(env.DB),
		SendingDomains:   sendingdomains.NewSecrets(env.DB),
		Users:            users.NewSecrets(env.DB),
		StaffMembers:     staffmembers.NewSecrets(env.DB),
		Automations:      automations.NewSecrets(env.DB),
		Rules:            rules.NewSecrets(env.DB),
	}, cipher)
}
