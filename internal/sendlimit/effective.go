package sendlimit

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
)

// Source says where an effective ceiling comes from.
type Source string

const (
	// SourceManual is the user-set value on the Integration.
	SourceManual Source = "manual"
	// SourceProvider is the value the provider reports (SES GetSendQuota).
	SourceProvider Source = "provider"
)

// Value is one effective ceiling: Limit is nil (and Source empty) when the window
// is not limited.
type Value struct {
	Limit  *int
	Source Source
}

// Effective is an Integration's ceilings as enforced: per second and per rolling
// 24 hours. Per ADR 0023 each is the minimum of the available values; when two
// sources give the same number the user's manual value is reported.
type Effective struct {
	PerSecond Value
	PerDay    Value

	// ProviderQuotaUnavailable is set while the provider quota lookup fails (the
	// Integration's provider_quota_failed_at is set), so
	// the ceilings above rest on the manual value alone (or on none).
	ProviderQuotaUnavailable bool
}

// EffectiveOf computes the ceilings an Integration is held to. It is the one place
// manual, provider (and later warmup) values are folded into a ceiling.
func EffectiveOf(integ *ent.Integration) Effective {
	return Effective{
		PerSecond:                lowest(integ.MaxPerSecond, integ.ProviderMaxPerSecond),
		PerDay:                   lowest(integ.MaxPerDay, integ.ProviderMaxPerDay),
		ProviderQuotaUnavailable: integ.ProviderQuotaFailedAt != nil,
	}
}

// lowest is the minimum rule: the lower of the two ceilings, the manual one on a tie.
func lowest(manual, provider *int) Value {
	switch {
	case manual == nil && provider == nil:
		return Value{}
	case provider == nil || (manual != nil && *manual <= *provider):
		return Value{Limit: manual, Source: SourceManual}
	default:
		return Value{Limit: provider, Source: SourceProvider}
	}
}

// Limits is what the token buckets enforce.
func (e Effective) Limits() Limits {
	return Limits{PerSecond: e.PerSecond.Limit, PerDay: e.PerDay.Limit}
}

// Unlimited reports whether no ceiling applies at all.
func (e Effective) Unlimited() bool { return !e.Limits().Any() }

// Warning names something a user should look at on an Integration's limit. The
// values are the API's warning enum, so each surface converts with a plain cast.
type Warning string

const (
	// WarningUnlimited: no ceiling applies, so sends are not paced.
	WarningUnlimited Warning = "unlimited"
	// WarningProviderQuotaUnavailable: the provider's quota could not be read.
	WarningProviderQuotaUnavailable Warning = "providerQuotaUnavailable"
)

// Warnings lists what is worth flagging about the ceilings, in a stable order; never nil.
func (e Effective) Warnings() []Warning {
	out := []Warning{}
	if e.Unlimited() {
		out = append(out, WarningUnlimited)
	}
	if e.ProviderQuotaUnavailable {
		out = append(out, WarningProviderQuotaUnavailable)
	}
	return out
}

// window is the trailing period Usage counts over.
const window = 24 * time.Hour

// Usage counts the messages each of the Workspace's Integrations sent over the 24
// hours before now, keyed by Integration id (an Integration that sent nothing is
// absent). It counts every Outbound message the provider accepted, Transactional
// included, because that is what the provider's own quota sees.
func Usage(ctx context.Context, s *ent.Scoped, now time.Time) (map[int64]int, error) {
	var rows []struct {
		IntegrationID int64 `json:"integration_id"`
		Count         int   `json:"count"`
	}
	err := s.OutboundMessage().Query().
		Where(
			outboundmessage.StatusEQ(outboundmessage.StatusSent),
			outboundmessage.SentAtGT(now.Add(-window)),
			outboundmessage.IntegrationIDNotNil(),
		).
		GroupBy(outboundmessage.FieldIntegrationID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, r := range rows {
		out[r.IntegrationID] = r.Count
	}
	return out, nil
}

// Status is an Integration's Send rate limit as shown to a user: the ceilings as
// enforced, the messages sent over the last 24 hours and what is worth flagging. Each
// surface maps it to its own DTO.
//
// A Value with a nil Limit has an empty Source (see Value), so no surface restates it.
type Status struct {
	PerSecond Value
	PerDay    Value
	Sent24h   int
	Warnings  []Warning
}

// Statuses pairs the 24-hour usage with the effective ceilings of each Integration,
// keyed by Integration id. Usage is read once for all rows.
func Statuses(ctx context.Context, s *ent.Scoped, rows []*ent.Integration, now time.Time) (map[int64]Status, error) {
	usage, err := Usage(ctx, s, now)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Status, len(rows))
	for _, row := range rows {
		eff := EffectiveOf(row)
		out[row.ID] = Status{
			PerSecond: eff.PerSecond,
			PerDay:    eff.PerDay,
			Sent24h:   usage[row.ID],
			Warnings:  eff.Warnings(),
		}
	}
	return out, nil
}
