import { expect, test } from 'vitest'

import { handleSiteWorkspacesList } from '../../../generated/site/msw.gen.ts'
import { Route as mcpRoute } from '../../../routes/workspaces/$workspaceId/settings/mcp.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { McpPage } from './mcp.tsx'

const MCP_ROUTE = routeMount(mcpRoute, { workspaceId: 'TestWs01' })

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  mcpUrl: 'https://mcp.sphericon.test/',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the MCP endpoint of the instance', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace] }))

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect
    .element(screen.getByRole('textbox', { name: 'Connector address' }))
    .toHaveValue('https://mcp.sphericon.test/mcp')
})

test('links to the MCP guide', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace] }))

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Other ways to connect' }))
    .toHaveAttribute('href', 'https://getsphericon.com/docs/guide/mcp')
})

test('tells a plain member that only an owner or admin can connect an agent', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }))

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect.element(screen.getByText(/ask an owner or admin/)).toBeInTheDocument()
})
