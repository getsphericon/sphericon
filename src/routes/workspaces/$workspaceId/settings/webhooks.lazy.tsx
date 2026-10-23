import { createLazyFileRoute } from '@tanstack/react-router'

import { WebhooksPage } from '../../../../pages/workspace/settings/webhooks.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/webhooks')({
  component: WebhooksPage,
})
