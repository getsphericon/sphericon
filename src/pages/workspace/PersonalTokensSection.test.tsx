import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSitePersonalTokensCreate,
  handleSitePersonalTokensDelete,
  handleSitePersonalTokensGrantableScopes,
  handleSitePersonalTokensList,
} from '../../generated/site/msw.gen.ts'
import type {
  SitePersonalTokenResource,
  SiteWorkspaceResource,
} from '../../generated/site/types.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { PersonalTokensSection } from './PersonalTokensSection.tsx'

const workspace: SiteWorkspaceResource = {
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
  role: 'member',
  createdAt: '2026-01-01T00:00:00Z',
}

const token: SitePersonalTokenResource = {
  id: '9',
  name: 'Import script',
  prefix: 'omk_pt12',
  scopes: ['contacts:read'],
  userId: '3',
  userName: 'Mary',
  userEmail: 'mary@acme.test',
  createdAt: '2026-06-01T00:00:00Z',
}

test('lists the personal tokens with whom each belongs to', async () => {
  worker.use(handleSitePersonalTokensList({ body: [token] }))

  const { screen } = await renderWithRouter(<PersonalTokensSection workspace={workspace} />)

  await expect.element(screen.getByText('Import script')).toBeInTheDocument()
  await expect.element(screen.getByText('Mary (mary@acme.test)')).toBeInTheDocument()
})

// The stubs below stand in for the server: the list answers what was minted or revoked since, so
// each test reads the outcome off the page.

test('mints a token with only the scopes the role may grant, reveals its secret once and lists it', async () => {
  let listed: SitePersonalTokenResource[] = []
  worker.use(
    handleSitePersonalTokensList(() => HttpResponse.json(listed)),
    handleSitePersonalTokensGrantableScopes({ body: ['contacts:read', 'contacts:write'] }),
    handleSitePersonalTokensCreate(async ({ request }) => {
      const input = await request.json()
      const minted = { ...token, name: input.name, scopes: input.scopes }
      listed = [minted]
      return HttpResponse.json(
        { token: 'omtk_pt12_supersecret', resource: minted },
        { status: 201 },
      )
    }),
  )

  const { screen } = await renderWithRouter(<PersonalTokensSection workspace={workspace} />)

  await expect.element(screen.getByText('No personal tokens')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Create personal token' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText(/^Token name/).fill('Nightly sync')
  await dialog.getByRole('checkbox', { name: 'read' }).click()
  await expect.element(dialog.getByRole('checkbox', { name: 'webhooks' })).not.toBeInTheDocument()
  await dialog.getByRole('button', { name: 'Create personal token' }).click()

  await expect.element(screen.getByText('omtk_pt12_supersecret')).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'Nightly sync' })).toBeInTheDocument()
  await expect.element(screen.getByText('1 scope')).toBeInTheDocument()
})

test('revokes a token after confirmation', async () => {
  let listed = [token]
  worker.use(
    handleSitePersonalTokensList(() => HttpResponse.json(listed)),
    handleSitePersonalTokensDelete(() => {
      listed = []
      return new HttpResponse(null, { status: 204 })
    }),
  )

  const { screen } = await renderWithRouter(<PersonalTokensSection workspace={workspace} />)

  await screen.getByRole('button', { name: 'Revoke Import script' }).click()
  await expect.element(screen.getByText('Revoke this personal token?')).toBeInTheDocument()
  await screen.getByRole('dialog').getByRole('button', { name: 'Revoke' }).click()

  await expect.element(screen.getByText('No personal tokens')).toBeInTheDocument()
  await expect.element(screen.getByText('Import script')).not.toBeInTheDocument()
})
