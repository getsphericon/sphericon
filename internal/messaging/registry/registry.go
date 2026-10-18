// Package registry assembles the default provider catalog. It is the single
// place that imports every provider impl, so the core messaging package stays
// free of provider dependencies (avoiding an import cycle). Register a new
// provider by adding its Descriptor() here.
package registry

import (
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
)

// New returns a catalog with all built-in providers registered. allowPrivateSMTP lets an
// SMTP config name a private or loopback host and any port; Workspace Integrations are
// customer input, so production passes false (see smtp.NewDescriptor).
func New(allowPrivateSMTP bool) *messaging.Catalog {
	return messaging.NewCatalog(
		smtp.NewDescriptor(allowPrivateSMTP),
		ses.Descriptor(),
	)
}

// Default is the catalog for destinations the staff member configured (the platform's own
// mail) and for tests: SMTP may reach private hosts.
func Default() *messaging.Catalog { return New(true) }
