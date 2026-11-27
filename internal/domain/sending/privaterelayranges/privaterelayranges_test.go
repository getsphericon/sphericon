package privaterelayranges_test

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	agents "github.com/monperrus/crawler-user-agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/sending/privaterelayranges"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// relayAddr is inside the PrivateRelayRange fixture.
const relayAddr = "198.51.100.77"

func cancelled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// detector is a detector over the fixtures. A test runs it inside a testing/synctest bubble, so
// testhelper.Elapse moves the detector's clock past the reload intervals at once.
func detector(t *testing.T) *privaterelayranges.Detector {
	t.Helper()
	env := testhelper.Setup(t)
	return privaterelayranges.New(env.DB, env.PrivateRelay, time.Now)
}

// A table that cannot be read again keeps serving the last one that could: a database that hiccups
// must not turn every Private Relay hit into a human open.
func TestAFailedReloadKeepsTheLastTable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := detector(t)
		assert.True(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, ClientAddr: relayAddr}), "the first read loads the table")

		testhelper.Elapse(2 * time.Hour) // the copy is stale, and the next read fails

		assert.True(t, d.IsMachine(cancelled(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, ClientAddr: relayAddr}))
	})
}

// With no table at all a failed read says nothing, and the read is not repeated on every hit.
func TestAFailedFirstReadIsTriedAgainAfterAWhile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := detector(t)
		assert.False(t, d.IsMachine(cancelled(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, ClientAddr: relayAddr}))

		assert.False(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, ClientAddr: relayAddr}), "not retried at once")

		testhelper.Elapse(2 * time.Minute)
		assert.True(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, ClientAddr: relayAddr}))
	})
}

// instancesOf is every user-agent the crawler list gives for pattern. The list is a library a
// dependency bump can rename: a pattern it no longer names stops an open through Gmail or Yahoo
// from being the person's again, so the test says so rather than passing with nothing to judge.
func instancesOf(t *testing.T, pattern string) []string {
	t.Helper()
	for i := range agents.Crawlers {
		crawler := &agents.Crawlers[i]
		if crawler.Pattern == pattern {
			require.NotEmpty(t, crawler.Instances, pattern)
			return crawler.Instances
		}
	}
	require.FailNow(t, "the crawler list no longer names "+pattern)
	return nil
}

// A mailbox provider's display proxy loads a message's images to show it, so an open through one is
// the person's; a click through the same proxy stays machine, since Yahoo's also fetches the URLs a
// message carries (ADR 0053).
func TestADisplayProxyOpenIsThePersonsAndItsClickIsMachine(t *testing.T) {
	d := detector(t)
	for _, pattern := range privaterelayranges.DisplayProxies {
		for _, userAgent := range instancesOf(t, pattern) {
			assert.False(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, UserAgent: userAgent}), pattern)
			assert.True(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AClick, UserAgent: userAgent}), pattern)
		}
	}
}

// crawlerInstances is every user-agent of the crawler list whose pattern is not one of skip. A list
// that leaves nothing to judge fails rather than passing with an empty loop.
func crawlerInstances(t *testing.T, skip []string) []string {
	t.Helper()
	var out []string
	for i := range agents.Crawlers {
		crawler := &agents.Crawlers[i]
		if slices.Contains(skip, crawler.Pattern) {
			continue
		}
		out = append(out, crawler.Instances...)
	}
	require.NotEmpty(t, out)
	return out
}

// Every crawler that is not a display proxy is machine on an open as on a click.
func TestAnotherCrawlerIsMachineOnAnOpenAndOnAClick(t *testing.T) {
	d := detector(t)
	for _, userAgent := range crawlerInstances(t, privaterelayranges.DisplayProxies) {
		assert.True(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AnOpen, UserAgent: userAgent}), userAgent)
		assert.True(t, d.IsMachine(t.Context(), privaterelayranges.IsMachineInput{Kind: privaterelayranges.AClick, UserAgent: userAgent}), userAgent)
	}
}
