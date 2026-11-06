package instancestates

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/visitor"
	"github.com/getsphericon/sphericon/internal/infra/postgres"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/workspaces"
)

// Figures are the facts about the whole Instance a Telemetry report summarizes (ADR 0032), read from
// core tables only: never from an Enterprise Usage snapshot, which a plain install does not have.
// The counts are exact; the report rounds them (internal/usecase/telemetryreport), and nothing here
// names a Workspace or a person.
type Figures struct {
	// State is the Instance state row: its Instance id and when the Instance first started.
	State *ent.InstanceState
	// Workspaces is the number of live Workspaces.
	Workspaces int
	// Contacts is the number of Contacts with an email address in live Workspaces.
	Contacts int
	// EmailsSent is the number of messages sent since the start of the window.
	EmailsSent int
	// UsedBroadcasts is whether a Broadcast was sent since the start of the window.
	UsedBroadcasts bool
	// AutomationsActive is whether an Automation is active.
	AutomationsActive bool
	// UsedTracker is whether a Visitor was seen since the start of the window.
	UsedTracker bool
	// UsedExternalAPI is whether an API token was used since the start of the window.
	UsedExternalAPI bool
	// WebhooksConfigured is whether a Webhook endpoint exists.
	WebhooksConfigured bool
	// Providers is the providers of the kept Integrations, each once.
	Providers []providers.Provider
	// PostgresMajor is the major version of the database server.
	PostgresMajor int
}

// Figures reads the Figures of the whole Instance, with since the start of the window of every
// "recent" figure. It creates the Instance state row on the first start, like Ensure.
func (m *Module) Figures(ctx context.Context, since time.Time) (Figures, error) {
	client := m.ent
	row, err := m.Ensure(ctx)
	if err != nil {
		return Figures{}, err
	}
	var q figureQueries
	f := Figures{State: row}
	f.Workspaces = q.count(client.Workspace.Query().Where(workspaces.Live()).Count(ctx))
	// Erasure deletes the Contact row (GLOSSARY: there is no softer delete), so an erased
	// Contact is already gone, and those of a Workspace in its deletion window are left out; anonymous tracker Contacts have no email and are left out.
	f.Contacts = q.count(client.Contact.Query().
		Where(contact.EmailNotNil(), contact.EmailNEQ(""), contact.HasWorkspaceWith(workspaces.Live())).Count(ctx))
	f.EmailsSent = q.count(client.OutboundMessage.Query().
		Where(outboundmessage.StatusEQ(outboundmessage.StatusSent), outboundmessage.SentAtGTE(since)).Count(ctx))
	f.UsedBroadcasts = q.exists(client.Broadcast.Query().
		Where(broadcast.StatusEQ(broadcast.StatusSent), broadcast.SentAtGTE(since)).Exist(ctx))
	f.AutomationsActive = q.exists(client.Automation.Query().Where(automation.StatusEQ(automation.StatusActive)).Exist(ctx))
	f.UsedTracker = q.exists(client.Visitor.Query().Where(visitor.LastSeenAtGTE(since)).Exist(ctx))
	f.UsedExternalAPI = q.exists(client.ApiToken.Query().Where(apitoken.LastUsedAtGTE(since)).Exist(ctx))
	f.WebhooksConfigured = q.exists(client.WebhookEndpoint.Query().Exist(ctx))
	// The providers in use are those of kept Integrations: a deleted one is not set up (ADR 0099).
	inUse, err := client.Integration.Query().Where(integration.DiscardedAtIsNil()).Unique(true).Select(integration.FieldProvider).Strings(ctx)
	if err != nil {
		return Figures{}, err
	}
	if q.err != nil {
		return Figures{}, q.err
	}
	f.PostgresMajor, err = postgres.ServerMajor(ctx, client)
	if err != nil {
		return Figures{}, err
	}
	for _, p := range inUse {
		f.Providers = append(f.Providers, providers.Provider(p))
	}
	return f, nil
}

// figureQueries keeps the first error of a run of reads, so Figures reads top to bottom.
type figureQueries struct{ err error }

func (q *figureQueries) count(n int, err error) int {
	q.err = firstErr(q.err, err)
	return n
}

func (q *figureQueries) exists(ok bool, err error) bool {
	q.err = firstErr(q.err, err)
	return ok
}

func firstErr(first, next error) error {
	if first != nil {
		return first
	}
	return next
}
