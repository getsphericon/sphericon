import { createFileRoute } from '@tanstack/react-router'

import { McpPage } from '../../../../pages/workspace/settings/mcp.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/mcp')({
  component: McpPage,
})
