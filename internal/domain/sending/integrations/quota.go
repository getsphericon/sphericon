package integrations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/qmuntal/stateless"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/ent/integrationproviderstate"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/internal/domain/sending/providerlimit"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/lifecycle"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// quotaTimeout bounds one provider quota lookup, so a hung endpoint cannot stall a
// save or a job.
const quotaTimeout = 15 * time.Second

// DueForQuotaRefresh returns up to limit Integrations whose provider quota is due a read, each
// with only its id and its Workspace (the lookup job names both), least-recently-checked first
// with NULLS FIRST, so a never-checked Integration is reached before already-checked ones. Only
// SES reports a quota; SMTP has none. A deleted Integration is not kept working (ADR 0099): nobody
// sends with its credential. The read crosses Workspaces, so it takes the job's raw client.
func (m *Module) DueForQuotaRefresh(ctx context.Context, client *ent.Client, limit int) ([]*ent.Integration, error) {
	// The moment of the last check is the Integration's provider state (ADR 0168): the read orders
	// the states and names each one's Integration.
	states, err := client.IntegrationProviderState.Query().
		Where(integrationproviderstate.HasIntegrationWith(integration.ProviderEQ(providers.SES), integration.DiscardedAtIsNil())).
		Order(integrationproviderstate.ByProviderQuotaCheckedAt(sql.OrderAsc(), sql.OrderNullsFirst()), integrationproviderstate.ByID()).
		Limit(limit).
		Select(integrationproviderstate.FieldIntegrationID, integrationproviderstate.FieldWorkspaceID).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*ent.Integration, len(states))
	for i, s := range states {
		out[i] = &ent.Integration{ID: s.IntegrationID, WorkspaceID: s.WorkspaceID}
	}
	return out, nil
}

// RefreshQuota asks the Integration's provider for its send quota and the account's
// standing and stores the answer beside the Integration, in its provider state (ADR 0067, ADR 0168): providerlimit.EffectiveOf
// folds the quota into the ceiling by the minimum rule, and a stopped account holds the
// Integration's sends. It runs after the Integration is saved and hourly from the periodic job,
// because SES quotas grow as an account matures and an account is stopped and resumed without
// notice.
//
// A failed lookup (no ses:GetAccount permission, an SES-compatible service such as
// Postbox, a timeout) never blocks sending and is not returned: it is recorded as
// the moment it first failed (provider_quota_failed_at), which the UI shows as a warning, and the last known
// provider values and standing stay, so a transient blip neither lifts a ceiling nor
// sends into a stopped account. A later success clears the warning. Providers that
// cannot report a quota (SMTP) are left untouched.
// An error is returned only when the stored config cannot be read or the row cannot
// be written.
func (m *Module) RefreshQuota(ctx context.Context, s *ent.Scoped, integ *ent.Integration) error {
	reader, ok, err := m.quotaReader(integ)
	if err != nil || !ok {
		return err
	}

	lookupCtx, cancel := context.WithTimeout(ctx, quotaTimeout)
	defer cancel()
	quota, lookupErr := reader.SendQuota(lookupCtx)

	now := m.now()
	if lookupErr != nil {
		slog.WarnContext(ctx, "provider send quota unavailable",
			"integration_id", integ.ID, "workspace_id", integ.WorkspaceID, "err", lookupErr)
		return recordQuotaUnavailable(ctx, s, integ.ID, now)
	}
	return recordQuota(ctx, s, integ.ID, quota, now)
}

// quotaReader is the sender of the Integration's provider when it can say what it allows (a
// messaging.QuotaReader); ok is false for a provider that cannot. An error is a stored config that
// cannot be read or built from.
func (m *Module) quotaReader(integ *ent.Integration) (reader messaging.QuotaReader, ok bool, err error) {
	if integ.Channel != channels.Email {
		return nil, false, nil
	}
	config, err := m.cipher.Decrypt(integ.ConfigEncrypted)
	if err != nil {
		return nil, false, fmt.Errorf("decrypt integration %d config: %w", integ.ID, err)
	}
	built, err := m.catalog.BuildEmail(integ.Provider, config, messaging.Deps{})
	if err != nil {
		return nil, false, err
	}
	reader, ok = messaging.AsQuotaReader(built)
	return reader, ok, nil
}

// recordQuotaUnavailable stores that the lookup for the Integration id failed: the moment it was
// checked, and the moment the lookup first failed, which a second failure keeps. The last known
// provider values and standing stay.
func recordQuotaUnavailable(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	if err := wroteState(stateOf(s, id).SetProviderQuotaCheckedAt(at).Save(ctx)); err != nil {
		return err
	}
	return markQuotaFailed(ctx, s, id, at)
}

// recordQuota stores the quota and standing the provider reported for the Integration id, with
// the moment it was checked, and ends a recorded failure.
func recordQuota(ctx context.Context, s *ent.Scoped, id int64, quota messaging.Quota, at time.Time) error {
	upd := stateOf(s, id).SetProviderQuotaCheckedAt(at)
	upd.SetNillableProviderMaxPerSecond(quota.PerSecond).
		SetNillableProviderMaxPerDay(quota.PerDay)
	if quota.PerSecond == nil {
		upd.ClearProviderMaxPerSecond()
	}
	if quota.PerDay == nil {
		upd.ClearProviderMaxPerDay()
	}
	if quota.Enforcement == providers.EnforcementNone {
		upd.ClearProviderEnforcement()
	} else {
		upd.SetProviderEnforcement(quota.Enforcement)
	}
	upd.SetProviderSandbox(quota.Sandbox)
	if err := wroteState(upd.Save(ctx)); err != nil {
		return err
	}
	return clearQuotaFailed(ctx, s, id)
}

// markQuotaFailed records the moment the lookup first failed: it writes only while no failure is
// recorded, so a second failed lookup keeps the first moment.
func markQuotaFailed(ctx context.Context, s *ent.Scoped, id int64, at time.Time) error {
	_, err := stateOf(s, id).
		Where(integrationproviderstate.ProviderQuotaFailedAtIsNil()).
		SetProviderQuotaFailedAt(at).
		Save(ctx)
	return err
}

// clearQuotaFailed ends a failure after a successful lookup; it writes only while one is recorded.
func clearQuotaFailed(ctx context.Context, s *ent.Scoped, id int64) error {
	_, err := stateOf(s, id).
		Where(integrationproviderstate.ProviderQuotaFailedAtNotNil()).
		ClearProviderQuotaFailedAt().
		Save(ctx)
	return err
}

// stateOf is the update of the provider state of the Integration id (ADR 0168).
func stateOf(s *ent.Scoped, id int64) *ent.IntegrationProviderStateScopedUpdate {
	return s.IntegrationProviderState().Update().Where(integrationproviderstate.IntegrationID(id))
}

// wroteState is the error of an unconditional write of one Integration's provider state: one that
// found no row is one, since every Integration has its row.
func wroteState(n int, err error) error {
	if err == nil && n != 1 {
		return fmt.Errorf("integrations: the write of a provider state found %d rows, not 1", n)
	}
	return err
}

// ProviderStanding records what a send learns of the provider's standing on an Integration, beside
// it in its provider state (ADR 0168): the role of Outbound send, which holds the clock and no
// Integrations module.
type ProviderStanding struct {
	now func() time.Time
}

// NewProviderStanding builds the role; now is the clock a refusal is dated by.
func NewProviderStanding(now func() time.Time) *ProviderStanding {
	return &ProviderStanding{now: now}
}

// RecordRefused stores, at the clock, that the provider refused the credentials integ was read
// with (provider_refused_at), which holds the Integration's Send route: Outbound send records it.
// The refusal is of those credentials only: one that lands after a save replaced them, or after
// another send recorded it, changes nothing.
func (p *ProviderStanding) RecordRefused(ctx context.Context, s *ent.Scoped, integ *ent.Integration) error {
	at := p.now()
	err := refusalMachine(lifecycle.Storage[refusal]{
		Read: func(context.Context) (refusal, error) { return refusalOf(integ), nil },
		Write: func(ctx context.Context, _, _ refusal) (bool, error) {
			n, err := stateOf(s, integ.ID).
				Where(append(judged(integ), integrationproviderstate.ProviderRefusedAtIsNil())...).
				SetProviderRefusedAt(at).
				Save(ctx)
			return n == 1, err
		},
	}).FireCtx(ctx, triggerRefuse)
	if errors.Is(err, lifecycle.ErrInvalidTransition) {
		return nil
	}
	return err
}

// renewCredentials lifts the provider's refusal of the credentials saved replaced, on the
// transaction s of the save: new credentials are tried at the next send. saved is the Integration as
// the save stored it.
func renewCredentials(ctx context.Context, s *ent.Scoped, saved *ent.Integration) error {
	return refusalMachine(lifecycle.Storage[refusal]{
		Read: func(context.Context) (refusal, error) { return refusalOf(saved), nil },
		Write: func(ctx context.Context, _, _ refusal) (bool, error) {
			n, err := stateOf(s, saved.ID).
				Where(heldWith(saved), integrationproviderstate.ProviderRefusedAtNotNil()).
				ClearProviderRefusedAt().
				Save(ctx)
			return n == 1, err
		},
	}).FireCtx(ctx, triggerRenew)
}

// judged matches the provider state of integ as the send that the provider refused read it: still
// in the moment it was read in, and of an Integration that still holds the credentials it was read
// with. A save of new credentials writes the state in its transaction (renewState), so a refusal of
// the credentials it replaced misses, whether the save committed before it or waits for it on the
// state's row. So does a refusal that lands after any other write of the state: the next send
// refused records it.
func judged(integ *ent.Integration) []predicate.IntegrationProviderState {
	return []predicate.IntegrationProviderState{
		integrationproviderstate.UpdatedAtEQ(providerlimit.StateOf(integ).UpdatedAt),
		heldWith(integ),
	}
}

// heldWith matches the provider state of an Integration that still holds the credentials integ was
// read with.
func heldWith(integ *ent.Integration) predicate.IntegrationProviderState {
	return integrationproviderstate.HasIntegrationWith(integration.ConfigEncrypted(integ.ConfigEncrypted))
}

// refusal is the provider's standing of an Integration's credentials, kept as provider_refused_at:
// a machine (ADR 0109). A send the provider refused holds the Send route, and a save of new
// credentials lifts it, so a background send races the Owner's save: each move is conditioned on
// the credentials it judged.
type refusal string

const (
	refusalAccepted refusal = "accepted"
	refusalRefused  refusal = "refused"
)

type refusalTrigger string

const (
	// triggerRefuse is a send the provider refused for its credentials.
	triggerRefuse refusalTrigger = "refuse"
	// triggerRenew is a save of new credentials.
	triggerRenew refusalTrigger = "renew"
)

func refusalOf(integ *ent.Integration) refusal {
	if providerlimit.Refused(integ) != nil {
		return refusalRefused
	}
	return refusalAccepted
}

// refusalMachine is the table of the credentials' standing: a refusal already recorded and new
// credentials of an Integration nothing refused change nothing.
func refusalMachine(storage lifecycle.Storage[refusal]) *stateless.StateMachine {
	m := lifecycle.New(storage)
	m.Configure(refusalAccepted).
		Permit(triggerRefuse, refusalRefused).
		Ignore(triggerRenew)
	m.Configure(refusalRefused).
		Permit(triggerRenew, refusalAccepted).
		Ignore(triggerRefuse)
	return m
}
