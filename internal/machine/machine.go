// Package machine tells an open or click made by software from one made by a person (ADR 0053,
// GLOSSARY: Machine engagement). A hit is machine when its user-agent belongs to a known crawler
// or scanner (the crawler-user-agents list, a maintained library) or its address lies inside an
// iCloud Private Relay egress range, a list Apple publishes and the refresh job keeps in a plain
// table. Detection is a partial signal: Apple publishes no list for Mail Privacy Protection and
// the Private Relay list is known to be incomplete. Neither the address nor the user-agent is
// kept: they are read for the verdict and dropped.
//
// The table holds no Workspace data, so the package holds the raw ent client.
package machine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/gaissmai/bart"
	agents "github.com/monperrus/crawler-user-agents"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/privaterelayrange"
)

// RangesURL is where Apple publishes the Private Relay egress ranges, one CSV row per range
// (`172.224.224.0/31,GB,GB-LND,London,`).
const RangesURL = "https://mask-api.icloud.com/egress-ip-ranges.csv"

const (
	// reloadAfter is how long the in-memory copy of the table serves before it is read again.
	reloadAfter = time.Hour
	// insertBatch bounds the rows of one INSERT while the table is replaced.
	insertBatch = 5000
	fetchLimit  = 64 << 20
)

// ErrNoRanges is a published list that held no range: it never replaces the stored one.
var ErrNoRanges = errors.New("machine: the published list holds no range")

// Source fetches the published Private Relay list. It is the seam a test replaces so that no
// test reaches the network.
type Source interface {
	Fetch(ctx context.Context) (io.ReadCloser, error)
}

// HTTPSource fetches the list from Apple over HTTPS.
type HTTPSource struct {
	URL    string
	Client *http.Client
}

// NewHTTPSource is the source of the composition root: Apple's published URL.
func NewHTTPSource() HTTPSource {
	return HTTPSource{URL: RangesURL, Client: &http.Client{Timeout: 2 * time.Minute}}
}

// Fetch implements Source.
func (s HTTPSource) Fetch(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("machine: ranges source answered %s", resp.Status)
	}
	return resp.Body, nil
}

// Detector judges a hit and refreshes the ranges it judges by.
type Detector struct {
	client *ent.Client
	source Source
	now    func() time.Time

	mu       sync.RWMutex
	ranges   *bart.Lite
	loadedAt time.Time
}

// New builds the detector over the raw client and the source of the published list.
func New(client *ent.Client, source Source) *Detector {
	return &Detector{client: client, source: source, now: time.Now}
}

// IsMachine reports whether a hit with this user-agent from this client address is machine
// engagement. An unreadable address or an empty user-agent says nothing: only a positive match
// flags a hit, so a person is never discounted on a guess.
func (d *Detector) IsMachine(ctx context.Context, userAgent, clientAddr string) bool {
	if userAgent != "" && agents.IsCrawler(userAgent) {
		return true
	}
	addr, err := netip.ParseAddr(clientAddr)
	if err != nil {
		return false
	}
	ranges, err := d.loaded(ctx)
	if err != nil {
		return false
	}
	return ranges.Lookup(addr.Unmap())
}

// loaded is the in-memory copy of the table, read again when it is older than reloadAfter.
func (d *Detector) loaded(ctx context.Context) (*bart.Lite, error) {
	d.mu.RLock()
	ranges, at := d.ranges, d.loadedAt
	d.mu.RUnlock()
	if ranges != nil && d.now().Sub(at) < reloadAfter {
		return ranges, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ranges != nil && d.now().Sub(d.loadedAt) < reloadAfter {
		return d.ranges, nil
	}
	rows, err := d.client.PrivateRelayRange.Query().Select(privaterelayrange.FieldCidr).Strings(ctx)
	if err != nil {
		return nil, err
	}
	table := new(bart.Lite)
	for _, cidr := range rows {
		if p, err := netip.ParsePrefix(cidr); err == nil {
			table.Insert(p)
		}
	}
	d.ranges, d.loadedAt = table, d.now()
	return table, nil
}

// Refresh replaces the stored ranges with the published list. A failure of any kind, a fetch
// error, an unreadable or empty list or a failed write, leaves the old ranges in place.
func (d *Detector) Refresh(ctx context.Context) error {
	body, err := d.source.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("machine: fetch ranges: %w", err)
	}
	defer func() { _ = body.Close() }()
	prefixes, err := parseRanges(io.LimitReader(body, fetchLimit))
	if err != nil {
		return err
	}
	if err := d.replace(ctx, prefixes); err != nil {
		return err
	}
	d.mu.Lock()
	d.ranges, d.loadedAt = nil, time.Time{} // read the new table on the next hit
	d.mu.Unlock()
	return nil
}

func (d *Detector) replace(ctx context.Context, prefixes []netip.Prefix) error {
	tx, err := d.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.PrivateRelayRange.Delete().Exec(ctx); err != nil {
		return err
	}
	for batch := range chunks(prefixes, insertBatch) {
		builders := make([]*ent.PrivateRelayRangeCreate, len(batch))
		for i, p := range batch {
			builders[i] = tx.PrivateRelayRange.Create().SetCidr(p.String())
		}
		if err := tx.PrivateRelayRange.CreateBulk(builders...).Exec(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func chunks[T any](s []T, n int) func(yield func([]T) bool) {
	return func(yield func([]T) bool) {
		for len(s) > 0 {
			k := min(n, len(s))
			if !yield(s[:k]) {
				return
			}
			s = s[k:]
		}
	}
}

// parseRanges reads the first column of each row as a CIDR range, masked and without
// duplicates. A row that is not a range is skipped; a list with no range at all is an error.
func parseRanges(r io.Reader) ([]netip.Prefix, error) {
	seen := map[netip.Prefix]struct{}{}
	var out []netip.Prefix
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		cidr, _, _ := strings.Cut(strings.TrimSpace(sc.Text()), ",")
		p, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		p = p.Masked()
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("machine: read ranges: %w", err)
	}
	if len(out) == 0 {
		return nil, ErrNoRanges
	}
	return out, nil
}
