package jobs

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// MessageObservationArgs deliberately carries no personal envelope: accepted measurements
// survive Erasure and only update the anonymous delivery record's report stamps.
type MessageObservationArgs struct {
	jobkind.Workspace

	Observation outboundmessages.Observation `json:"observation"`
}

func (MessageObservationArgs) Kind() string { return "event_message_observation" }
func (MessageObservationArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: events.QueueEvents}
}

func observesMessage(env events.Envelope, ev events.DomainEvent) bool {
	_, ok := outboundmessages.ObservationOf(env, ev)
	return ok
}
func observeMessage(env events.Envelope, ev events.DomainEvent) river.JobArgs {
	observation, _ := outboundmessages.ObservationOf(env, ev)
	return MessageObservationArgs{Workspace: jobkind.Workspace{WorkspaceID: env.WorkspaceID}, Observation: observation}
}

type MessageObservationWorker struct {
	river.WorkerDefaults[MessageObservationArgs]

	ent *ent.Client
}

func NewMessageObservationWorker(client *ent.Client) *MessageObservationWorker {
	return &MessageObservationWorker{ent: client}
}
func (w *MessageObservationWorker) Work(ctx context.Context, job *river.Job[MessageObservationArgs]) error {
	return outboundmessages.RecordObservation(ctx, w.ent.Scoped(job.Args.WorkspaceID), job.Args.Observation)
}
