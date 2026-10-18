package server

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/secrets"
	sns "github.com/robbiet480/go.sns"
)

// NewSESHooks builds the SES hook handler with the SNS signature check and the
// subscription confirmation replaced, so tests can drive real notification
// payloads without AWS-signed messages or network access.
func NewSESHooks(client *ent.Client, bus *events.Bus, cipher *secrets.Cipher, verify func(*sns.Payload) error, confirm func(context.Context, string) error) http.Handler {
	h := &sesHook{ent: client, bus: bus, topics: integrations.New(bus, cipher, nil, nil), verify: verify, confirm: confirm}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{key}/ses", h.handle)
	return mux
}

// ConfirmSNSSubscription exposes the real SubscribeURL guard.
var ConfirmSNSSubscription = confirmSNSSubscription
