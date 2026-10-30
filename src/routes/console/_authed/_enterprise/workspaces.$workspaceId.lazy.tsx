import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleWorkspacePage } from '../../../../console/workspace.tsx'

export const Route = createLazyFileRoute('/console/_authed/_enterprise/workspaces/$workspaceId')({
  component: ConsoleWorkspacePage,
})
