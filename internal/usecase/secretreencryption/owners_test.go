package secretreencryption_test

import (
	"slices"

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

// reencryption builds the use case the way the wiring does, on the owners' columns of stored
// secrets over the test's raw client, with cipher as the target.
func reencryption(env *testhelper.TestEnv, cipher *secrets.Cipher) *secretreencryption.UseCase {
	return secretreencryption.New(slices.Concat(
		integrations.SecretSweeps(env.DB), webhookendpoints.SecretSweeps(env.DB), sendingdomains.SecretSweeps(env.DB),
		users.SecretSweeps(env.DB), staffmembers.SecretSweeps(env.DB), automations.SecretSweeps(env.DB), rules.SecretSweeps(env.DB),
	), cipher)
}
