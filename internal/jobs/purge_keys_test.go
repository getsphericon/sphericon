package jobs_test

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// argFields is the json names of the args' fields, the embedded ones' included, with their kind.
func argFields(t reflect.Type) map[string]reflect.Kind {
	fields := map[string]reflect.Kind{}
	for f := range t.Fields() {
		if f.Anonymous {
			maps.Copy(fields, argFields(f.Type))
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		fields[name] = f.Type.Kind()
	}
	return fields
}

// Erasure finds the queued jobs that hold a person's data by a purge key's argument, which river
// stores under the json name of the args' field: the key names that field only by its string, so a
// field renamed on either side would match no job and leave the data queued. Every purged kind
// is the Kind of one of the args below, and its argument is an integer field of them.
func TestEveryPurgeKeyNamesAnIntegerArgumentOfItsJob(t *testing.T) {
	byKind := map[string]river.JobArgs{}
	for _, args := range []river.JobArgs{
		jobs.DeliverWebhookArgs{}, jobs.EvaluateTriggerArgs{}, jobs.SendRecipientArgs{}, jobs.EvaluateRulesArgs{},
		automations.RunStepArgs{}, automations.RunHTTPStepArgs{},
	} {
		byKind[args.Kind()] = args
	}

	for _, key := range jobkind.Purged {
		args, ok := byKind[key.Kind]
		require.True(t, ok, "no job args of the purged kind %q", key.Kind)
		fields := argFields(reflect.TypeOf(args))
		assert.Equal(t, reflect.Int64, fields[key.Arg], "the purge argument of %q is not an int64 field of its args", key.Kind)
	}
	assert.Equal(t, reflect.Slice, argFields(reflect.TypeFor[jobs.DeliverWebhookArgs]())[jobkind.DeliverWebhookBodyArg],
		"a queued webhook delivery's body is not where the Erasure clears it")
}
