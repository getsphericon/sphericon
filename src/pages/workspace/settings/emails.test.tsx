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

test('shows the Send cap the platform set, apart from the Provider limit', async () => {
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

  await expect.element(screen.getByRole('heading', { name: 'Send cap' })).toBeInTheDocument()
  await expect.element(screen.getByText('5,000 per 24 hours')).toBeInTheDocument()
  await expect.element(screen.getByText('Set by the platform')).toBeInTheDocument()
  await expect.element(screen.getByText('Sent in the last 24 h: 1,200 / 5,000')).toBeInTheDocument()
})

test('shows no Send cap when none applies', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, sendCap: null }] }),
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
  await expect.element(screen.getByRole('heading', { name: 'Send cap' })).not.toBeInTheDocument()
  await expect.element(screen.getByText('Set by the platform')).not.toBeInTheDocument()
})
