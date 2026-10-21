import { expect, test } from 'vitest'

import {
  handleSiteMembershipsList,
  handleSiteUserGetMe,
  handleSiteTokensList,
  handleSiteWebhooksList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import { Route as developersRoute } from '../../../routes/workspaces/$workspaceId/settings/developers.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { DevelopersPage } from './developers.tsx'

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the API keys and the webhooks of the workspace', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteTokensList({ body: [] }),
    handleSiteUserGetMe({
      body: {
        id: '10',
        name: 'Ann',
        email: 'ann@acme.com',
        emailVerified: true,
        createdAt: '2026-01-01T00:00:00Z',
      },
    }),
    handleSiteMembershipsList({
      body: [
        {
          id: '1',
          userId: '10',
          email: 'ann@acme.com',
          name: 'Ann',
          role: 'owner',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    }),
    handleSiteWebhooksList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
  )

  const { screen } = await renderWithRouter(
    <DevelopersPage />,
    routeMount(developersRoute, { workspaceId: 'TestWs01' }),
  )

  await expect.element(screen.getByRole('heading', { name: 'API keys' })).toBeInTheDocument()
  await expect.element(screen.getByRole('heading', { name: 'Webhooks' })).toBeInTheDocument()
})

test('does not offer a member the webhooks, whose secrets only owners and admins read', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }),
    handleSiteTokensList({ body: [] }),
    handleSiteUserGetMe({
      body: {
        id: '10',
        name: 'Ann',
        email: 'ann@acme.com',
        emailVerified: true,
        createdAt: '2026-01-01T00:00:00Z',
      },
    }),
    handleSiteMembershipsList({
      body: [
        {
          id: '1',
          userId: '10',
          email: 'ann@acme.com',
          name: 'Ann',
          role: 'member',
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
    }),
  )

  const { screen } = await renderWithRouter(
    <DevelopersPage />,
    routeMount(developersRoute, { workspaceId: 'TestWs01' }),
  )

  await expect.element(screen.getByRole('heading', { name: 'API keys' })).toBeInTheDocument()
  await expect.element(screen.getByRole('heading', { name: 'Webhooks' })).not.toBeInTheDocument()
})
