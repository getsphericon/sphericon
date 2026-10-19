import { expect, test } from 'vitest'

import { handleSiteWorkspacesList } from '../../../generated/site/msw.gen.ts'
import { Route as mcpRoute } from '../../../routes/workspaces/$slug/settings/mcp.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { McpPage } from './mcp.tsx'

const MCP_ROUTE = routeMount(mcpRoute, { slug: 'test' })

const workspace = {
  id: '1',
  name: 'Acme',
  slug: 'test',
  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://mcp.sphericon.test/',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the MCP endpoint of the instance', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace] }))

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect.element(screen.getByText('https://mcp.sphericon.test/mcp')).toBeInTheDocument()
})

test('links to the MCP guide', async () => {
  worker.use(handleSiteWorkspacesList({ body: [workspace] }))

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Other ways to connect' }))
    .toHaveAttribute('href', 'https://docs.getsphericon.app/guide/mcp')
})

test('tells a plain member that only an owner or admin can connect an agent', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }))

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect.element(screen.getByText(/ask an owner or admin/)).toBeInTheDocument()
})
