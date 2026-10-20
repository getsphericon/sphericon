import { createFileRoute } from '@tanstack/react-router'

import { SendingPage } from '../../../../pages/workspace/settings/sending.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/sending')({
  component: SendingPage,
})
