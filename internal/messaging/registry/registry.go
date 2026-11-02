// Package registry assembles the default provider catalog. It is the single
// place that imports every provider impl, so the core messaging package stays
// free of provider dependencies (avoiding an import cycle). Register a new
// provider by adding its Descriptor() here.
package registry

import (
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/inbox"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/messaging/telegram"
)

// New returns a catalog with all built-in providers registered. allowPrivateSMTP lets an
// SMTP config name a private or loopback host and any port; Workspace Integrations are
// customer input, so production passes false (see smtp.NewDescriptor). telegramAPIURL is the
// Bot API base URL every Telegram bot is reached at (SPHERICON_TELEGRAM_BOT_API_URL).
func New(allowPrivateSMTP bool, telegramAPIURL string) *messaging.Catalog {
	return messaging.NewCatalog(
		smtp.NewDescriptor(allowPrivateSMTP),
		ses.Descriptor(),
		inbox.Descriptor(),
		telegram.Descriptor(telegramAPIURL),
	)
}

// Default is the catalog for destinations the self-hoster configured (the platform's own
// mail) and for tests: SMTP may reach private hosts, and Telegram is Telegram's own server.
func Default() *messaging.Catalog { return New(true, telegram.DefaultAPIURL) }
