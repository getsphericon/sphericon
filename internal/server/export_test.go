package server

import (
	"context"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/integrations"
	sns "github.com/robbiet480/go.sns"
)

// NewHooks builds the hook handler with the SNS signature check and the subscription
// confirmation replaced, so tests can drive real SES notification payloads without
// AWS-signed messages or network access.
func NewHooks(client *ent.Client, bus *events.Bus, integ *integrations.Module, verify func(*sns.Payload) error, confirm func(context.Context, string) error) http.Handler {
	h := &hook{ent: client, bus: bus, integ: integ, now: time.Now, verifySNS: verify, confirmURL: confirm}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{key}", h.handle)
	return mux
}

// ConfirmSNSSubscription exposes the real SubscribeURL guard.
var ConfirmSNSSubscription = confirmSNSSubscription
