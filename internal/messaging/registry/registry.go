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

// Default returns a catalog with all built-in providers registered.
func Default() *messaging.Catalog {
	return messaging.NewCatalog(
		smtp.Descriptor(),
		ses.Descriptor(),
	)
}
