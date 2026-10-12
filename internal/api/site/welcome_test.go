package site_test

import (
	"context"
	"strings"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Registration enqueues platform emails (a welcome email and an email-verification
// link). The inline jobs adapter runs them synchronously through the system
// sender, so we assert the real sends — not just that jobs were enqueued.
func TestSiteRegisterSendsWelcomeEmail(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)
	ctx := context.Background()

	res, err := c.SiteAuthRegister(ctx, &siteapi.SiteRegisterInput{
		Name:     "Grace Hopper",
		Email:    siteapi.EmailAddress("grace@example.com"),
		Password: "s3cret-password",
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteRegisterResult{}, res)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 2, "registration sends a welcome and a verification email")

	subjects := []string{msgs[0].Subject, msgs[1].Subject}
	for _, m := range msgs {
		assert.Equal(t, "grace@example.com", m.To)
	}
	assert.Contains(t, strings.Join(subjects, "\n"), "Welcome")
	assert.Contains(t, strings.Join(subjects, "\n"), "Verify")
}
