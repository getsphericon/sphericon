import { expect, test } from 'vitest'

import {
  handleSiteMembershipsList,
  handleSiteUserGetMe,
  handleSiteIntegrationsList,
  handleSiteSendingDomainsList,
  handleSiteSuppressionsList,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import { Route as sendingRoute } from '../../../routes/workspaces/$slug/settings/sending.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { SendingPage } from './sending.tsx'

const workspace = {
  id: '1',
  name: 'Acme',
  slug: 'test',
  publicId: 'AcmeWs01',
  collectKey: 'omck_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the providers, the sending domains and the suppression list', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteIntegrationsList({ body: [] }),
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
    handleSiteSendingDomainsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
    handleSiteSuppressionsList({
      body: { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 1 },
    }),
  )

  const { screen } = await renderWithRouter(
    <SendingPage />,
    routeMount(sendingRoute, { slug: 'test' }),
  )

  await expect.element(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
  await expect.element(screen.getByRole('heading', { name: 'Domains' })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('heading', { name: 'Suppression list' }))
    .toBeInTheDocument()
})
