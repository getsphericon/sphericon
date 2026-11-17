package callbackinterpretation_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/event"
	"github.com/getsphericon/sphericon/internal/domain/contacts/eligibility"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/domain/sending/callbackinterpretation"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSimultaneousTelegramSTOPCallbacksConsumeOneObservation(t *testing.T) {
	env := testhelper.Setup(t, testhelper.Committed())
	scope := env.DB.Scoped(fixtures.AcmeID)
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	t.Cleanup(unblock)
	var first sync.Once
	env.Bus.UseSubscribers(events.Subscriber{
		Wants: func(_ events.Envelope, ev events.DomainEvent) bool {
			_, ok := ev.(*events.TelegramStopObserved)
			return ok
		},
		Handle: func(context.Context, *ent.Scoped, events.Publisher, events.Envelope, events.DomainEvent) error {
			first.Do(func() { close(entered); <-release })
			return nil
		},
	})
	bot, err := scope.Integration().Get(t.Context(), fixtures.IntegrationAcmeTelegramID)
	require.NoError(t, err)
	u := testhelper.Module[*callbackinterpretation.Module](env)
	in := callbackinterpretation.InterpretInput{Integration: bot, Preset: "telegram", Result: preset.Result{
		InboundMessages: []preset.InboundMessage{{ID: "1", Channel: "telegram", Destination: fixtures.BindingLiamDestination, Text: "/stop"}},
	}}
	done := make(chan error, 1)
	go func() { done <- u.Interpret(t.Context(), scope, in) }()
	testhelper.Receive(t, entered, "the first observation holds its receipt and consent change")
	results := env.QueueBehind(t, unblock, testhelper.Writer{Name: "duplicate", Run: func() error {
		return u.Interpret(t.Context(), scope, in)
	}})
	require.NoError(t, testhelper.Receive(t, done, "first callback commits"))
	require.NoError(t, results["duplicate"])
	stopped, err := unsubscribes.New().Get(t.Context(), scope, unsubscribes.Key{Channel: channels.Telegram, Destination: fixtures.BindingLiamDestination, Source: eligibility.SourceEverything})
	require.NoError(t, err)
	require.NotNil(t, stopped)
	transitions := scope.Event().Query().Where(event.Action(systemevent.TelegramUnsubscribed), event.ContactID(fixtures.ContactLiamID)).CountX(t.Context())
	assert.Equal(t, 1, transitions)
}
