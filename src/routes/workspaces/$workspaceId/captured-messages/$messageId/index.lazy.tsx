import { createLazyFileRoute } from '@tanstack/react-router'

import { CapturedMessagePage } from '../../../../../pages/captured-messages/detail.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/captured-messages/$messageId/')({
  component: CapturedMessagePage,
})
