import { createFileRoute } from '@tanstack/react-router'

import { EmailsPage } from '../../../../pages/workspace/settings/emails.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/emails')({
  component: EmailsPage,
})
