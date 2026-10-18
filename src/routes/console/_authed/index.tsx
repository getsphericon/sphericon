import { createFileRoute } from '@tanstack/react-router'

import { ConsoleWorkspacesPage } from '../../../console/workspaces.tsx'

export const Route = createFileRoute('/console/_authed/')({
  component: ConsoleWorkspacesPage,
})
