package messaging

import (
	"context"
	"errors"
	"fmt"
)

// SMSSender is the SMS Channel's provider: one Workspace's own account at an SMS provider (Twilio),
// sending from the sender its Integration names. Connecting an Integration finds that sender with
// the credentials and points the provider's callbacks at the Integration's Ingest hook.
type SMSSender interface {
	// Sender reads the configured sender with the credentials: a read the provider answers only
	// for credentials it accepts, so it checks them too. Credentials it refuses are an error
	// wrapping ErrSMSUnauthorized; a sender the account does not have, ErrSMSSenderNotFound.
	Sender(ctx context.Context) (SMSFrom, error)
	// PointCallbacks points the sender's inbound messages, and the status callbacks where the
	// provider keeps them on the sender, at url. It reports false, with no error, when the provider
	// keeps no callback on this kind of sender or the credentials may not change it: the address is
	// then given to the provider some other way (with each message, or by hand).
	PointCallbacks(ctx context.Context, from SMSFrom, url string) (bool, error)
	// Send hands one text to the provider, from the configured sender. A refusal of the provider's
	// API is an error wrapping a *SMSError; a network error or a 5xx is a plain error, which a
	// retry may get past.
	Send(ctx context.Context, msg SMSMessage) (SMSSent, error)
}

// SMSMessage is one text to one number: the number in E.164, the text as it goes out, and where
// the provider posts the message's delivery reports (the Integration's Ingest hook). The provider
// keeps a status callback only on a Messaging Service, so every send names it.
type SMSMessage struct {
	To             string
	Body           string
	StatusCallback string
}

// SMSSent is what the provider answers to an accepted message: its id of the message, enough on
// its own to find it again (Twilio's message SID).
type SMSSent struct {
	ID string
}

// SMSFromKind is what a message goes out from.
type SMSFromKind string

const (
	// SMSFromNumber is a phone number of the account, in E.164.
	SMSFromNumber SMSFromKind = "number"
	// SMSFromAlphanumeric is an alphanumeric sender ID: one-way, no reply reaches it.
	SMSFromAlphanumeric SMSFromKind = "alphanumeric"
	// SMSFromMessagingService is a Messaging Service of the account, which picks the number.
	SMSFromMessagingService SMSFromKind = "messagingService"
)

// SMSFrom is the sender as the provider knows it: its kind and the provider's id of it (a number's
// or a Messaging Service's SID; empty for an alphanumeric sender ID).
type SMSFrom struct {
	Kind SMSFromKind
	SID  string
}

// ErrSMSUnauthorized means the provider does not accept the credentials: wrong, revoked, or of a
// closed account.
var ErrSMSUnauthorized = errors.New("sms: the provider does not accept the credentials")

// ErrSMSSenderNotFound means the account has no such sender.
var ErrSMSSenderNotFound = errors.New("sms: the account has no such sender")

// SMSError is a refusal of the provider's API: the HTTP status, the provider's error code and its
// message in its own words.
type SMSError struct {
	Status  int
	Code    int
	Message string
}

func (e *SMSError) Error() string {
	return fmt.Sprintf("sms: %d (code %d) %s", e.Status, e.Code, e.Message)
}

// Is makes a 401 match ErrSMSUnauthorized and a 404 ErrSMSSenderNotFound.
func (e *SMSError) Is(target error) bool {
	switch target {
	case ErrSMSUnauthorized:
		return e.Status == 401
	case ErrSMSSenderNotFound:
		return e.Status == 404
	}
	return false
}
