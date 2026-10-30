import { expect, test } from 'vitest'

import {
  handleSiteWebhooksList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import { Route as webhooksRoute } from '../../../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { WebhooksPage } from './webhooks.tsx'

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the webhooks of the workspace', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteWebhooksList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
  )

  const { screen } = await renderWithRouter(
    <WebhooksPage />,
    routeMount(webhooksRoute, { workspaceId: 'TestWs01' }),
  )

  await expect
    .element(screen.getByRole('heading', { name: 'Webhooks', level: 4 }))
    .toBeInTheDocument()
})

test('does not offer a member the webhooks, whose secrets only owners and admins read', async () => {
  worker.use(handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member' as const }] }))

  const { screen } = await renderWithRouter(
    <WebhooksPage />,
    routeMount(webhooksRoute, { workspaceId: 'TestWs01' }),
  )

  await expect
    .element(screen.getByRole('heading', { name: 'Webhooks', level: 2 }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('heading', { name: 'Webhooks', level: 4 }))
    .not.toBeInTheDocument()
})
