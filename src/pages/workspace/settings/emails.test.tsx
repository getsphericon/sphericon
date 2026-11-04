import { expect, test } from 'vitest'

import {
  handleSiteAuditList,
  handleSiteIntegrationsList,
  handleSiteSendingDomainsList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import type { SiteIntegrationResource } from '../../../generated/site/types.gen.ts'
import { Route as emailsRoute } from '../../../routes/workspaces/$workspaceId/settings/emails.tsx'
import { problem } from '../../../test/problem.ts'
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
  shortLinkLength: 35,
  smsMaxLength: { gsm7: 1600, ucs2: 700 },
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

const telegramBot: SiteIntegrationResource = {
  id: '8',
  name: 'News bot',
  channel: 'telegram',
  provider: 'telegram',
  enabled: true,
  isDefault: true,
  maxPerSecond: null,
  maxPerDay: null,
  providerLimit: {
    perSecond: { limit: 30, source: 'provider' },
    perDay: { limit: null, source: null },
    sentLast24h: 0,
    warnings: [],
  },
  config: { kind: 'telegram', botId: 7000000001, username: 'acme_news_bot' },
  preset: 'telegram',
  params: { secret: '********' },
  secretsRetiring: [],
  hookUrl: 'https://app.test/hooks/omik_bot',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}

test('a link to an Integration opens the form that saves its credentials', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [{ ...workspace, permissions: ['integrations:manage'] }] }),
    handleSiteIntegrationsList({ body: [telegramBot] }),
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
    handleSiteAuditList(() => problem(402)),
  )
  const mount = routeMount(emailsRoute, { workspaceId: 'TestWs01' })

  const { screen, navigate } = await renderWithRouter(<EmailsPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?integration=%228%22`,
  })

  const dialog = screen.getByRole('dialog', { name: 'Credentials: News bot' })
  await expect.element(dialog.getByLabelText(/^Bot token/)).toBeInTheDocument()
  await dialog.getByRole('button', { name: 'Cancel' }).click()

  expect(navigate).toHaveBeenCalledWith(expect.objectContaining({ search: {}, replace: true }))
})
