import { createLazyFileRoute } from '@tanstack/react-router'

import { CapturedMessagesListPage } from '../../../../pages/captured-messages/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/captured-messages/')({
  component: CapturedMessagesListPage,
})
