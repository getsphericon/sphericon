package smtp

import (
	"fmt"
	"net"
	"syscall"
	"time"

	"code.dny.dev/ssrf"
)

// publicDialer refuses, after DNS resolution, an address that is not on the public internet, so a
// hostname that resolves (or rebinds) to an internal address is refused before any packet is sent.
// The ranges are IANA's special-purpose registries, kept by code.dny.dev/ssrf, and the port is one
// of the mail ports a config may name.
func publicDialer() *net.Dialer {
	guard := ssrf.New(ssrf.WithPorts(mailPorts...))
	return &net.Dialer{
		Timeout: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			if err := guard.Safe(network, address, c); err != nil {
				return fmt.Errorf("smtp: refusing to connect to non-public address %s: %w", address, err)
			}
			return nil
		},
	}
}
