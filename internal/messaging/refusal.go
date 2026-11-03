package messaging

import "errors"

// ErrRefused says the provider refuses the send for a reason no retry changes: the credentials
// may not send as the From identity, the identity or its domain is not verified, the message is
// rejected. It is the opposite of ErrBusy and ErrQuotaExceeded, which wait for capacity: a
// refusal is the same answer every time until a person changes the account, so a job queue
// that gets it stops retrying and says so. An adapter wraps it around the provider's own error.
var ErrRefused = errors.New("provider refuses the send")
