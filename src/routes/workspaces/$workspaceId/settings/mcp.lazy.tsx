import { createLazyFileRoute } from '@tanstack/react-router'

import { McpPage } from '../../../../pages/workspace/settings/mcp.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/mcp')({
  component: McpPage,
})
