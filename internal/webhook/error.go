package webhook

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/doyensec/safeurl"
)

// ErrorKind is the closed set of reasons a delivery fails (ADR 0039). It is the
// vocabulary an owner, the API and an agent read; prose never carries it.
type ErrorKind string

const (
	// KindTimeout: the receiver did not answer within the delivery timeout.
	KindTimeout ErrorKind = "timeout"
	// KindBlockedAddress: the URL names an address the Instance does not permit
	// (private, loopback, link-local, or a scheme or port the client refuses).
	KindBlockedAddress ErrorKind = "blocked_address"
	// KindHTTPStatus: the receiver answered with a non-2xx status (DeliveryError.Status).
	KindHTTPStatus ErrorKind = "http_status"
	// KindNetwork: any other transport failure (DNS, refused or reset connection).
	KindNetwork ErrorKind = "network"
)

// DeliveryError is a failed delivery with its closed kind. Status is set only for
// KindHTTPStatus. RetryAfter is the receiver's Retry-After header, verbatim, when it sent one
// (ParseRetryAfter reads it).
type DeliveryError struct {
	Kind       ErrorKind
	Status     int
	RetryAfter string
	// Excerpt is the first ExcerptBytes of the receiver's answer to a KindHTTPStatus failure.
	Excerpt string
	cause   error
}

func (e *DeliveryError) Error() string {
	if e.Kind == KindHTTPStatus {
		return fmt.Sprintf("webhook endpoint returned status %d", e.Status)
	}
	return fmt.Sprintf("webhook delivery failed (%s): %v", e.Kind, e.cause)
}

func (e *DeliveryError) Unwrap() error { return e.cause }

func statusError(status int, retryAfter string) *DeliveryError {
	return &DeliveryError{Kind: KindHTTPStatus, Status: status, RetryAfter: retryAfter}
}

// ParseRetryAfter reads a Retry-After value (RFC 9110: delta-seconds or an HTTP-date) as the
// wait from now. ok is false for an absent or unreadable value; a date already past is a
// zero wait.
func ParseRetryAfter(value string, now time.Time) (wait time.Duration, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.ParseUint(value, 10, 31); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if at, err := http.ParseTime(value); err == nil {
		return max(at.Sub(now), 0), true
	}
	return 0, false
}

// transportError classifies an error from the client's Do.
func transportError(err error) *DeliveryError {
	var (
		port   *safeurl.AllowedPortError
		scheme *safeurl.AllowedSchemeError
		host   *safeurl.InvalidHostError
		allowH *safeurl.AllowedHostError
		ip     *safeurl.AllowedIPError
		v6     *safeurl.IPv6BlockedError
		creds  *safeurl.SendingCredentialsBlockedError
		netErr net.Error
	)
	switch {
	case errors.As(err, &port), errors.As(err, &scheme), errors.As(err, &host),
		errors.As(err, &allowH), errors.As(err, &ip), errors.As(err, &v6), errors.As(err, &creds):
		return &DeliveryError{Kind: KindBlockedAddress, cause: err}
	case errors.As(err, &netErr) && netErr.Timeout():
		return &DeliveryError{Kind: KindTimeout, cause: err}
	}
	return &DeliveryError{Kind: KindNetwork, cause: err}
}

// RateLimited returns the receiver's 429 answer, or nil for any other outcome. A 429 is
// neither a success nor a failure: the receiver is alive and asking us to slow down.
func RateLimited(sendErr error) *DeliveryError {
	var deliveryErr *DeliveryError
	if errors.As(sendErr, &deliveryErr) && deliveryErr.Kind == KindHTTPStatus &&
		deliveryErr.Status == http.StatusTooManyRequests {
		return deliveryErr
	}
	return nil
}
