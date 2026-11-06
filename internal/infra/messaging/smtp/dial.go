package smtp

import (
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"time"
)

// publicOnly is checked by the dialer on the address it is about to connect to, after DNS
// resolution, so a hostname that resolves (or rebinds) to an internal address is refused.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"), // NAT64 can embed a private IPv4
}

func publicDialer() *net.Dialer {
	return &net.Dialer{
		Timeout: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			addr, err := netip.ParseAddrPort(address)
			if err != nil {
				return fmt.Errorf("smtp: refusing %q: %w", address, err)
			}
			if !isPublic(addr.Addr().Unmap()) {
				return fmt.Errorf("smtp: refusing to connect to non-public address %s", addr.Addr())
			}
			return nil
		},
	}
}

func isPublic(ip netip.Addr) bool {
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}
