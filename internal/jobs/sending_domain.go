package jobs

import (
	"context"
	stdsql "database/sql"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/internal/domainverify"
	"github.com/getsphericon/sphericon/internal/joindomains"
)

// recheckBatchSize bounds how many domains one periodic tick re-checks, so a
// large workspace population can't stall the tick. Domains are picked
// least-recently-checked first, so every domain is reached over successive ticks.
const recheckBatchSize = 100

// --- on-demand single-domain verification (the "Verify" button) ---

type VerifySendingDomainArgs struct {
	SendingDomainID int64 `json:"sending_domain_id"`
}

func (VerifySendingDomainArgs) Kind() string { return "sending_domain_verify" }

type VerifySendingDomainWorker struct {
	river.WorkerDefaults[VerifySendingDomainArgs]

	mod *domainverify.Module
}

func (w *VerifySendingDomainWorker) Work(ctx context.Context, job *river.Job[VerifySendingDomainArgs]) error {
	return w.mod.Verify(ctx, job.Args.SendingDomainID)
}

// --- periodic re-check of all domains (verified is a live property, ADR 0010) ---

type RecheckSendingDomainsArgs struct{}

func (RecheckSendingDomainsArgs) Kind() string { return "sending_domain_recheck" }

type RecheckSendingDomainsWorker struct {
	river.WorkerDefaults[RecheckSendingDomainsArgs]

	mod *domainverify.Module
}

// Work fans the periodic tick out into one VerifySendingDomain job per due domain, giving each
// check its own isolation and retry and reusing the path of the "Verify" button.
func (w *RecheckSendingDomainsWorker) Work(ctx context.Context, _ *river.Job[RecheckSendingDomainsArgs]) error {
	ids, err := w.mod.Due(ctx, recheckBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifySendingDomainArgs{SendingDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// --- periodic re-check of Tracking domains (independent of the DKIM re-check) ---

type VerifyTrackingDomainArgs struct {
	SendingDomainID int64 `json:"sending_domain_id"`
}

func (VerifyTrackingDomainArgs) Kind() string { return "tracking_domain_verify" }

type VerifyTrackingDomainWorker struct {
	river.WorkerDefaults[VerifyTrackingDomainArgs]

	mod *domainverify.Module
}

func (w *VerifyTrackingDomainWorker) Work(ctx context.Context, job *river.Job[VerifyTrackingDomainArgs]) error {
	return w.mod.VerifyTracking(ctx, job.Args.SendingDomainID)
}

type RecheckTrackingDomainsArgs struct{}

func (RecheckTrackingDomainsArgs) Kind() string { return "tracking_domain_recheck" }

type RecheckTrackingDomainsWorker struct {
	river.WorkerDefaults[RecheckTrackingDomainsArgs]

	mod *domainverify.Module
}

// Work fans the tick out into one VerifyTrackingDomain job per due Sending domain.
func (w *RecheckTrackingDomainsWorker) Work(ctx context.Context, _ *river.Job[RecheckTrackingDomainsArgs]) error {
	ids, err := w.mod.TrackingDue(ctx)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifyTrackingDomainArgs{SendingDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}

// --- periodic re-check of verified Join domains (the same shape as the Sending domain one) ---

type VerifyJoinDomainArgs struct {
	JoinDomainID int64 `json:"join_domain_id"`
}

func (VerifyJoinDomainArgs) Kind() string { return "join_domain_verify" }

type VerifyJoinDomainWorker struct {
	river.WorkerDefaults[VerifyJoinDomainArgs]

	mod *joindomains.Module
}

// Work re-checks one domain. The Owners' mail is sent by the module, best-effort.
func (w *VerifyJoinDomainWorker) Work(ctx context.Context, job *river.Job[VerifyJoinDomainArgs]) error {
	_, err := w.mod.Recheck(ctx, job.Args.JoinDomainID)
	return err
}

type RecheckJoinDomainsArgs struct{}

func (RecheckJoinDomainsArgs) Kind() string { return "join_domain_recheck" }

type RecheckJoinDomainsWorker struct {
	river.WorkerDefaults[RecheckJoinDomainsArgs]

	mod *joindomains.Module
}

// Work fans the tick out into one VerifyJoinDomain job per due domain, up to
// recheckBatchSize, least-recently-checked first.
func (w *RecheckJoinDomainsWorker) Work(ctx context.Context, _ *river.Job[RecheckJoinDomainsArgs]) error {
	ids, err := w.mod.Due(ctx, recheckBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, id := range ids {
		if _, err := rc.Insert(ctx, VerifyJoinDomainArgs{JoinDomainID: id}, nil); err != nil {
			return err
		}
	}
	return nil
}
