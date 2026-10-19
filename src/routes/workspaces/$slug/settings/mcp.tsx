import { createFileRoute } from '@tanstack/react-router'

import { McpPage } from '../../../../pages/workspace/settings/mcp.tsx'

export const Route = createFileRoute('/workspaces/$slug/settings/mcp')({
  component: McpPage,
})
