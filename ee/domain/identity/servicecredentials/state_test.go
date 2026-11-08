package servicecredentials_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ee/domain/identity/servicecredentials"
	"github.com/getsphericon/sphericon/ent"
)

var stateNow = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

func TestACredentialIsActiveUntilItsExpiry(t *testing.T) {
	future := stateNow.Add(time.Hour)

	expiring := servicecredentials.StateAt(&ent.ServiceCredential{ExpiresAt: &future}, stateNow)
	forever := servicecredentials.StateAt(&ent.ServiceCredential{}, stateNow)

	assert.Equal(t, servicecredentials.StateActive, expiring)
	assert.Equal(t, servicecredentials.StateActive, forever)
}

func TestACredentialIsExpiredFromItsExpiryOn(t *testing.T) {
	state := servicecredentials.StateAt(&ent.ServiceCredential{ExpiresAt: &stateNow}, stateNow)

	assert.Equal(t, servicecredentials.StateExpired, state)
}

func TestARevokedCredentialIsRevokedWhateverItsExpiry(t *testing.T) {
	past, future := stateNow.Add(-time.Hour), stateNow.Add(time.Hour)

	state := servicecredentials.StateAt(&ent.ServiceCredential{RevokedAt: &past, ExpiresAt: &future}, stateNow)

	assert.Equal(t, servicecredentials.StateRevoked, state)
}
