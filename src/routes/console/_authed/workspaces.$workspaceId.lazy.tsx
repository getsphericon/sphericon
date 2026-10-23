import { createLazyFileRoute } from '@tanstack/react-router'

import { ConsoleWorkspacePage } from '../../../console/workspace.tsx'

export const Route = createLazyFileRoute('/console/_authed/workspaces/$workspaceId')({
  component: ConsoleWorkspacePage,
})
