import { expect, test } from 'vitest'

import {
  handleSiteMembershipsList,
  handleSiteUserGetMe,
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
    handleSiteUserGetMe({
      body: {
        id: '10',
        name: 'Ann',
        email: 'ann@acme.com',
        emailVerified: true,
        signupSurveyDue: false,
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
  )

  const { screen } = await renderWithRouter(
    <EmailsPage />,
    routeMount(emailsRoute, { workspaceId: 'TestWs01' }),
  )

  await expect.element(screen.getByRole('heading', { name: 'Providers' })).toBeInTheDocument()
  await expect.element(screen.getByRole('heading', { name: 'Domains' })).toBeInTheDocument()
})
