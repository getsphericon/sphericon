package devdns_test

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/infra/devdns"
)

// owning is a source that answers records at name only.
func owning(name string, records ...string) devdns.Source {
	return func(_ context.Context, asked string) ([]string, error) {
		if asked != name {
			return nil, nil
		}
		return records, nil
	}
}

func TestLookupAnswersFromTheSourceThatOwnsTheName(t *testing.T) {
	lookup := devdns.Lookup(owning("a.test.", "one"), owning("b.test.", "two", "three"))

	records, err := lookup(t.Context(), "b.test.")

	require.NoError(t, err)
	assert.Equal(t, []string{"two", "three"}, records)
}

func TestLookupIsNotFoundWhenNoSourceHasARecord(t *testing.T) {
	lookup := devdns.Lookup(owning("a.test.", "one"))

	_, err := lookup(t.Context(), "c.test.")

	var dnsErr *net.DNSError
	require.ErrorAs(t, err, &dnsErr)
	assert.True(t, dnsErr.IsNotFound)
}

func TestLookupReturnsASourcesFault(t *testing.T) {
	fault := errors.New("store down")
	lookup := devdns.Lookup(func(context.Context, string) ([]string, error) { return nil, fault }, owning("a.test.", "one"))

	_, err := lookup(t.Context(), "a.test.")

	require.ErrorIs(t, err, fault)
}
