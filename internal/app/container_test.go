package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/samber/do/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/app"
	"github.com/getsphericon/sphericon/internal/bindings"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// What queues work never reaches the job client: Integrations and bindings, then Outbound send,
// which takes both, build without the job client or its pool. Were the queue the job client,
// whose workers drive Outbound send, the modules would need each other and the container could
// not build them in this order.
func TestQueueingWorkNeedsNoJobClient(t *testing.T) {
	baseline(t)
	cfg, err := config.Load("test")
	require.NoError(t, err)
	injector := app.Container(cfg)
	t.Cleanup(func() { cleanup.Shutdown(injector.ShutdownWithContext(context.Background())) })

	_, err = do.Invoke[*integrations.Module](injector)
	require.NoError(t, err)
	_, err = do.Invoke[*bindings.Module](injector)
	require.NoError(t, err)
	_, err = do.Invoke[*outbound.Module](injector)
	require.NoError(t, err)

	for _, s := range injector.ListInvokedServices() {
		assert.False(t, strings.HasSuffix(s.Service, ".jobsClient") || strings.HasSuffix(s.Service, ".riverPool"),
			"%s was built", s.Service)
	}
}
