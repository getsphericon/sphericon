import { createFileRoute } from '@tanstack/react-router'

import { WebhooksPage } from '../../../../pages/workspace/settings/webhooks.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/webhooks')({
  component: WebhooksPage,
})
