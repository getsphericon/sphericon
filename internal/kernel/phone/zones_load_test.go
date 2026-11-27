package phone_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/phone"
	"github.com/getsphericon/sphericon/internal/kernel/reportday"
)

// Every zone the Sending window may judge a number by loads (ADR 0134): a zone that fails to load
// is an error of the send, retried for ever, so the zone database must know the zones of every
// country's numbers whatever the host has installed.
func TestEveryZoneANumberMayBeInLoads(t *testing.T) {
	for _, number := range exampleNumbers() {
		for _, zone := range phone.Zones(number) {
			_, err := reportday.Load(zone)
			assert.NoError(t, err, "%s of %s", zone, number)
		}
	}
}
