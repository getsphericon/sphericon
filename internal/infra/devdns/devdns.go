// Package devdns is the TXT resolver of the end-to-end suite: no test domain publishes a real
// record, so each module that proves a domain by DNS feeds it the records its own rows imply, and
// a domain a scenario adds verifies without DNS. It knows no module and no record format: the
// modules' Sources answer the names they own.
package devdns

import (
	"context"
	"net"

	"github.com/getsphericon/sphericon/internal/kernel/domainname"
)

// Source answers the TXT records it implies for name, a fully qualified record name: none for a
// name it does not own or has no record at. An error is a fault of its own store.
type Source func(ctx context.Context, name string) ([]string, error)

// Lookup resolves a name to the records of the first source that has any, and to NXDOMAIN when
// none has.
func Lookup(sources ...Source) domainname.TXTLookup {
	return func(ctx context.Context, name string) ([]string, error) {
		for _, source := range sources {
			records, err := source(ctx, name)
			if err != nil {
				return nil, err
			}
			if len(records) > 0 {
				return records, nil
			}
		}
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
}
