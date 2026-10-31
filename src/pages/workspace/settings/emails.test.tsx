import { expect, test } from 'vitest'

import {
  handleSiteIntegrationsList,
  handleSiteSendingDomainsList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import { Route as emailsRoute } from '../../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { EmailsPage } from './emails.tsx'

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
  sendCap: null,
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the providers and the sending domains', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
  )

  const { screen } = await renderWithRouter(
    <EmailsPage />,
    routeMount(emailsRoute, { workspaceId: 'TestWs01' }),
  )

  await expect.element(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
  await expect.element(screen.getByRole('heading', { name: 'Domains' })).toBeInTheDocument()
})

test('a Workspace with a Send cap sees it, set by the platform, with its last-24-hour sends', async () => {
  worker.use(
    handleSiteWorkspacesList({
      body: [{ ...workspace, sendCap: { perDay: 5000, sentLast24h: 1200 } }],
    }),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
  )

  const { screen } = await renderWithRouter(
    <EmailsPage />,
    routeMount(emailsRoute, { workspaceId: 'TestWs01' }),
  )

  await expect
    .element(screen.getByText(/Send cap: 5 000 per 24 hours, set by the platform/))
    .toBeInTheDocument()
  await expect.element(screen.getByText(/1 200 \/ 5 000/)).toBeInTheDocument()
  await expect
    .element(screen.getByRole('progressbar', { name: 'Sent in the last 24 h' }))
    .toBeInTheDocument()
})

test('a Workspace without a Send cap sees none', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
  )

  const { screen } = await renderWithRouter(
    <EmailsPage />,
    routeMount(emailsRoute, { workspaceId: 'TestWs01' }),
  )

  await expect.element(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
  await expect.element(screen.getByText(/Send cap/)).not.toBeInTheDocument()
})
