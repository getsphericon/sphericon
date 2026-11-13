package callbackinterpretation_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/channelstop"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/sending/callbackinterpretation"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSimultaneousTelegramSTOPCallbacksConsumeOneObservation(t *testing.T) {
	r := testhelper.NewWorkspaceRace(t, fixtures.AcmeID)
	r.Bus.UseSubscribers(channelstop.Subscribers()...)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var first sync.Once
	r.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			_, ok := ev.(*events.TelegramStopObserved)
			return ok
		},
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			first.Do(func() { close(entered); <-release })
			return nil
		},
	})
	bot, err := r.Scope.Integration().Get(t.Context(), fixtures.IntegrationAcmeTelegramID)
	require.NoError(t, err)
	integs := integrations.New(r.Bus, testhelper.Cipher(t), nil, nil, "", time.Now)
	u := callbackinterpretation.New(entclient.Core.New(r.DB), r.Bus, integs, time.Now)
	in := callbackinterpretation.InterpretInput{Integration: bot, Preset: "telegram", Result: preset.Result{
		InboundMessages: []preset.InboundMessage{{ID: "1", Channel: "telegram", Destination: fixtures.BindingLiamDestination, Text: "/stop"}},
	}}
	done := make(chan error, 1)
	go func() { done <- u.Interpret(t.Context(), r.Scope, in) }()
	testhelper.Receive(t, entered, "the first observation holds its receipt and consent change")
	results := testhelper.QueuedBehind(t, r.DB, unblock, testhelper.Writer{Name: "duplicate", Run: func() error {
		return u.Interpret(t.Context(), r.Scope, in)
	}})
	require.NoError(t, testhelper.Receive(t, done, "first callback commits"))
	require.NoError(t, results["duplicate"])
	stopped, err := unsubscribes.Get(t.Context(), r.Scope, channels.Telegram, fixtures.BindingLiamDestination, eligibility.SourceEverything)
	require.NoError(t, err)
	require.NotNil(t, stopped)
	transitions := r.Scope.Event().Query().Where(event.Action(systemevent.TelegramUnsubscribed), event.ContactID(fixtures.ContactLiamID)).CountX(t.Context())
	assert.Equal(t, 1, transitions)
}
