import { expect, test } from 'vitest'

import {
  handleSiteWebhooksList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { Route as webhooksRoute } from '../../../routes/workspaces/$workspaceId/settings/webhooks.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { WebhooksPage } from './webhooks.tsx'

const workspace: SiteWorkspaceResource = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: ['webhooks:manage'],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner',
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

test('does not offer the webhooks, whose secrets are read with webhooks:manage, to anyone without it', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, role: 'member', permissions: [] }] }),
  )

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
