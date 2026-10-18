package webhook

import (
	"errors"
	"fmt"
	"net"

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
// KindHTTPStatus.
type DeliveryError struct {
	Kind   ErrorKind
	Status int
	cause  error
}

func (e *DeliveryError) Error() string {
	if e.Kind == KindHTTPStatus {
		return fmt.Sprintf("webhook endpoint returned status %d", e.Status)
	}
	return fmt.Sprintf("webhook delivery failed (%s): %v", e.Kind, e.cause)
}

func (e *DeliveryError) Unwrap() error { return e.cause }

func statusError(status int) *DeliveryError {
	return &DeliveryError{Kind: KindHTTPStatus, Status: status}
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
