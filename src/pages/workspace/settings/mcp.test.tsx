import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteConnectionsDelete,
  handleSiteConnectionsList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
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
  permissions: [],
  mcpUrl: 'https://mcp.sphericon.test/',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const connection = {
  id: '7',
  name: 'Claude',
  scopes: ['contacts:read' as const],
  userId: '3',
  userName: 'Mary',
  userEmail: 'mary@acme.test',
  createdAt: '2026-01-02T00:00:00Z',
}

test('shows the MCP endpoint of the instance', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteConnectionsList({ body: [] }),
  )

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect
    .element(screen.getByRole('textbox', { name: 'Connector address' }))
    .toHaveValue('https://mcp.sphericon.test/mcp')
})

test('links to the MCP guide', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteConnectionsList({ body: [] }),
  )

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect
    .element(screen.getByRole('link', { name: 'Other ways to connect' }))
    .toHaveAttribute('href', 'https://getsphericon.com/docs/guide/mcp')
})

test('lists the connected applications with the member each belongs to', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteConnectionsList({ body: [connection] }),
  )

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)

  await expect.element(screen.getByText('Claude', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Mary (mary@acme.test)')).toBeInTheDocument()
})

test('revoking a connection asks first, then removes it', async () => {
  const revoked: string[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteConnectionsList({ body: [connection] }),
    handleSiteConnectionsDelete(({ params }) => {
      revoked.push(params.id)
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<McpPage />, MCP_ROUTE)
  await screen.getByRole('button', { name: 'Revoke Claude' }).click()
  await screen.getByRole('button', { name: 'Revoke', exact: true }).click()

  await expect.poll(() => revoked).toEqual(['7'])
})
