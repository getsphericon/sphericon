import { createFileRoute } from '@tanstack/react-router'

import { ConsoleWorkspacePage } from '../../../console/workspace.tsx'

export const Route = createFileRoute('/console/_authed/workspaces/$workspaceId')({
  component: ConsoleWorkspacePage,
})
