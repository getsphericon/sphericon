package testhelper

import (
	"sync"

	"github.com/getsphericon/sphericon/internal/credentials"
)

// Passwords is the password hasher of the test process: the minimum argon2id cost, as the
// fixtures are hashed. A test knows its passwords and checks only that a hash verifies; the
// server's profile spends 64 MiB and tens of milliseconds a hash, on every sign-up, reset and
// password change a test makes.
var Passwords = sync.OnceValue(credentials.NewMinimumCostHasher)
