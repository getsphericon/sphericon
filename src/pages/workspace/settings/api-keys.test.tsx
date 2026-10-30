import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteMembershipsList,
  handleSiteUserGetMe,
  handleSiteTokensList,
  handleSiteTokensUpdate,
  handleSiteWorkspacesList,
} from '../../../generated/site/msw.gen.ts'
import { Route as apiKeysRoute } from '../../../routes/workspaces/$workspaceId/settings/api-keys.tsx'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { routeMount } from '../../../test/routeMount.ts'
import { worker } from '../../../test/worker.ts'
import { ApiKeysPage } from './api-keys.tsx'

const workspace = {
  id: 'TestWs01',
  name: 'Acme',

  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: ['tokens:manage' as const],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

test('shows the API keys of the workspace', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteTokensList({ body: [] }),
    handleSiteUserGetMe({
      body: {
        id: '10',
        name: 'Ann',
        email: 'ann@acme.com',
        emailVerified: true,
        signupSurveyDue: false,
        acceptancesPending: [],
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
  )

  const { screen } = await renderWithRouter(
    <ApiKeysPage />,
    routeMount(apiKeysRoute, { workspaceId: 'TestWs01' }),
  )

  await expect
    .element(screen.getByRole('heading', { name: 'API keys', level: 4 }))
    .toBeInTheDocument()
})

test('renames an API key from its row menu', async () => {
  const token = {
    id: '7',
    name: 'CI',
    prefix: 'omk_ab12',
    scopes: ['contacts:read' as const],
    createdAt: '2026-01-01T00:00:00Z',
  }
  const renamed: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteTokensList({ body: [token] }),
    handleSiteTokensUpdate(async ({ request }) => {
      renamed.push(await request.json())
      return HttpResponse.json({ ...token, name: 'Deploys' })
    }),
  )

  const { screen } = await renderWithRouter(
    <ApiKeysPage />,
    routeMount(apiKeysRoute, { workspaceId: 'TestWs01' }),
  )

  await screen.getByRole('button', { name: 'More actions' }).click()
  await screen.getByRole('menuitem', { name: 'Rename' }).click()
  await screen.getByLabelText(/^Token name/).fill('Deploys')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => renamed).toEqual([{ name: 'Deploys' }])
})
