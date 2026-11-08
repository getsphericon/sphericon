package jobs

import (
	"context"
	stdsql "database/sql"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/joindomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/sendingdomains"
	"github.com/getsphericon/sphericon/internal/domain/sending/trackingdomains"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// recheckBatchSize bounds how many domains one periodic tick re-checks, so a
// large workspace population can't stall the tick. Domains are picked
// least-recently-checked first, so every domain is reached over successive ticks.
const recheckBatchSize = 100

// --- on-demand single-domain verification (the "Verify" button) ---

type VerifySendingDomainArgs struct {
	jobkind.Workspace

	SendingDomainID int64 `json:"sending_domain_id"`
}

func (VerifySendingDomainArgs) Kind() string { return "sending_domain_verify" }

type VerifySendingDomainWorker struct {
	river.WorkerDefaults[VerifySendingDomainArgs]

	ent     *ent.Client
	domains *sendingdomains.Module
}

// NewVerifySendingDomainWorker builds the worker that verifies one Sending domain.
func NewVerifySendingDomainWorker(client *ent.Client, domains *sendingdomains.Module) *VerifySendingDomainWorker {
	return &VerifySendingDomainWorker{ent: client, domains: domains}
}

// Work reads the domain by id to learn its Workspace, before any scope exists, and checks it in
// that Workspace's scope. An unknown id is an error.
func (w *VerifySendingDomainWorker) Work(ctx context.Context, job *river.Job[VerifySendingDomainArgs]) error {
	dom, err := w.ent.SendingDomain.Get(ctx, job.Args.SendingDomainID)
	if err != nil {
		return err
	}
	return w.domains.Recheck(ctx, w.ent.Scoped(dom.WorkspaceID), dom.ID)
}

// --- periodic re-check of all domains (verified is a live property, ADR 0010) ---

type RecheckSendingDomainsArgs struct{}

func (RecheckSendingDomainsArgs) Kind() string { return "sending_domain_recheck" }

type RecheckSendingDomainsWorker struct {
	river.WorkerDefaults[RecheckSendingDomainsArgs]

	ent *ent.Client
}

// NewRecheckSendingDomainsWorker builds the worker that queues a check of every Sending domain due one.
func NewRecheckSendingDomainsWorker(client *ent.Client) *RecheckSendingDomainsWorker {
	return &RecheckSendingDomainsWorker{ent: client}
}

// Work fans the periodic tick out into one VerifySendingDomain job per due domain, giving each
// check its own isolation and retry and reusing the path of the "Verify" button.
func (w *RecheckSendingDomainsWorker) Work(ctx context.Context, _ *river.Job[RecheckSendingDomainsArgs]) error {
	due, err := sendingdomains.Due(ctx, w.ent, recheckBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, row := range due {
		args := VerifySendingDomainArgs{WorkspaceID: row.WorkspaceID, SendingDomainID: row.ID}
		if _, err := rc.Insert(ctx, args, nil); err != nil {
			return err
		}
	}
	return nil
}

// --- Tracking domains (ADR 0071): one check per verified host, retired ones included ---

type VerifyTrackingDomainArgs struct {
	jobkind.Workspace

	TrackingDomainID int64 `json:"tracking_domain_id"`
}

func (VerifyTrackingDomainArgs) Kind() string { return "tracking_domain_verify" }

type VerifyTrackingDomainWorker struct {
	river.WorkerDefaults[VerifyTrackingDomainArgs]

	mod *trackingdomains.Module
}

// NewVerifyTrackingDomainWorker builds the worker that checks one Tracking domain.
func NewVerifyTrackingDomainWorker(mod *trackingdomains.Module) *VerifyTrackingDomainWorker {
	return &VerifyTrackingDomainWorker{mod: mod}
}

func (w *VerifyTrackingDomainWorker) Work(ctx context.Context, job *river.Job[VerifyTrackingDomainArgs]) error {
	return w.mod.Recheck(ctx, job.Args.TrackingDomainID)
}

type RecheckTrackingDomainsArgs struct{}

func (RecheckTrackingDomainsArgs) Kind() string { return "tracking_domain_recheck" }

type RecheckTrackingDomainsWorker struct {
	river.WorkerDefaults[RecheckTrackingDomainsArgs]

	mod *trackingdomains.Module
}

// NewRecheckTrackingDomainsWorker builds the worker that queues a check of every verified tracking host.
func NewRecheckTrackingDomainsWorker(mod *trackingdomains.Module) *RecheckTrackingDomainsWorker {
	return &RecheckTrackingDomainsWorker{mod: mod}
}

// Work fans the tick out into one VerifyTrackingDomain job per verified host.
func (w *RecheckTrackingDomainsWorker) Work(ctx context.Context, _ *river.Job[RecheckTrackingDomainsArgs]) error {
	due, err := w.mod.Due(ctx)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, row := range due {
		args := VerifyTrackingDomainArgs{WorkspaceID: row.WorkspaceID, TrackingDomainID: row.ID}
		if _, err := rc.Insert(ctx, args, nil); err != nil {
			return err
		}
	}
	return nil
}

// --- periodic re-check of verified Join domains (the same shape as the Sending domain one) ---

type VerifyJoinDomainArgs struct {
	jobkind.Workspace

	JoinDomainID int64 `json:"join_domain_id"`
}

func (VerifyJoinDomainArgs) Kind() string { return "join_domain_verify" }

type VerifyJoinDomainWorker struct {
	river.WorkerDefaults[VerifyJoinDomainArgs]

	mod *joindomains.Module
}

// NewVerifyJoinDomainWorker builds the worker that verifies one Join domain.
func NewVerifyJoinDomainWorker(mod *joindomains.Module) *VerifyJoinDomainWorker {
	return &VerifyJoinDomainWorker{mod: mod}
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

// NewRecheckJoinDomainsWorker builds the worker that queues a check of every Join domain due one.
func NewRecheckJoinDomainsWorker(mod *joindomains.Module) *RecheckJoinDomainsWorker {
	return &RecheckJoinDomainsWorker{mod: mod}
}

// Work fans the tick out into one VerifyJoinDomain job per due domain, up to
// recheckBatchSize, least-recently-checked first.
func (w *RecheckJoinDomainsWorker) Work(ctx context.Context, _ *river.Job[RecheckJoinDomainsArgs]) error {
	due, err := w.mod.Due(ctx, recheckBatchSize)
	if err != nil {
		return err
	}
	rc := river.ClientFromContext[*stdsql.Tx](ctx)
	for _, row := range due {
		args := VerifyJoinDomainArgs{WorkspaceID: row.WorkspaceID, JoinDomainID: row.ID}
		if _, err := rc.Insert(ctx, args, nil); err != nil {
			return err
		}
	}
	return nil
}
