import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleWorkspacesPage } from '../../../../console/workspaces.tsx'

export const Route = createLazyFileRoute('/console/_authed/_enterprise/workspaces/')({
  component: ConsoleWorkspacesPage,
})
