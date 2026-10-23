import { createLazyFileRoute } from '@tanstack/react-router'

import { EmailsPage } from '../../../../pages/workspace/settings/emails.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/emails')({
  component: EmailsPage,
})
