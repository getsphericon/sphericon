import { HttpResponse } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'

import {
  handleSiteAuthLogout,
  handleSiteWorkspacesAccountLink,
  handleSiteWorkspacesListDeleted,
  handleSiteWorkspacesRestore,
} from '../generated/site/msw.gen.ts'
import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { UserMenu } from './UserMenu.tsx'

// Opening the menu asks for the deleted workspaces; most tests have none.
beforeEach(() => {
  worker.use(handleSiteWorkspacesListDeleted({ body: [] }))
})

test('logout calls the site logout operation and navigates to login', async () => {
  let calls = 0
  worker.use(
    handleSiteAuthLogout(() => {
      calls += 1
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)

  await screen.getByRole('button').click()
  await screen.getByText('Sign out').click()

  await expect.poll(() => calls).toBe(1)
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/login' }])
})

test('shows workspace settings only when a workspace is active', async () => {
  const { screen } = await renderWithRouter(<UserMenu />)
  await screen.getByRole('button').click()

  await expect.element(screen.getByText('Sign out')).toBeInTheDocument()
  expect(screen.container.textContent).not.toContain('Workspace settings')
})

test('the user block links to the profile page', async () => {
  const { screen } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)
  await screen.getByRole('button').click()

  await expect
    .element(screen.getByRole('menuitem', { name: /Test User/ }))
    .toHaveAttribute('href', routeMount(profileRoute).initialPath)
})

test('workspace settings item links with the active workspaceId', async () => {
  const { screen } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)
  await screen.getByRole('button').click()

  await expect
    .element(screen.getByRole('menuitem', { name: 'Workspace settings' }))
    .toHaveAttribute('href', routeMount(settingsRoute, { workspaceId: 'AcmeWs01' }).initialPath)
})

test('a failed logout shows an error and stays put', async () => {
  worker.use(handleSiteAuthLogout(() => HttpResponse.error()))
  const { screen, navigate } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)
  await screen.getByRole('button').click()
  await screen.getByText('Sign out').click()

  await expect.element(screen.getByText('Sign out failed').first()).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('shows the label and its initial in the avatar when given', async () => {
  const { screen } = await renderWithRouter(<UserMenu label="ada@example.com" />)

  await expect.element(screen.getByText('ada@example.com')).toBeInTheDocument()
  await expect.element(screen.getByText('A', { exact: true })).toBeInTheDocument()
})

test('the account link item is absent unless the workspace offers it', async () => {
  const { screen } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)
  await screen.getByRole('button', { expanded: false }).click()

  await expect.element(screen.getByText('Sign out')).toBeInTheDocument()
  expect(screen.container.textContent).not.toContain('Account')
})

test('a failed account link shows an error and opens nothing', async () => {
  worker.use(handleSiteWorkspacesAccountLink(() => HttpResponse.error()))
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const { screen } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" accountLink />)
  await screen.getByRole('button', { expanded: false }).click()
  await screen.getByText('Account', { exact: true }).click()

  await expect.element(screen.getByText('Could not open the account page')).toBeInTheDocument()
  expect(open).not.toHaveBeenCalled()
  open.mockRestore()
})

test('the account link item opens the external page with the token', async () => {
  worker.use(
    handleSiteWorkspacesAccountLink({
      body: {
        url: 'https://accounts.example.test/open',
        token: 'signed.jwt.token',
        expiresAt: '2026-05-01T12:02:00Z',
      },
    }),
  )
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const { screen } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" accountLink />)
  await screen.getByRole('button', { expanded: false }).click()
  await screen.getByText('Account', { exact: true }).click()

  await expect
    .poll(() => open.mock.calls)
    .toContainEqual([
      'https://accounts.example.test/open?token=signed.jwt.token',
      '_blank',
      'noopener',
    ])
  open.mockRestore()
})

const deletedInitech = {
  id: 'InitcWs3',
  name: 'Initech',
  discardedAt: '2026-02-01T00:00:00Z',
  eraseAt: '2026-03-03T00:00:00Z',
  onlyOwner: false,
}

test('lists the deleted workspaces with their erase date and restores one', async () => {
  const restored: string[] = []
  worker.use(
    handleSiteWorkspacesListDeleted({ body: [deletedInitech] }),
    handleSiteWorkspacesRestore(({ params }) => {
      restored.push(params.id)
      return HttpResponse.json({
        id: 'InitcWs3',
        name: 'Initech',
        collectKey: 'omck_test_key',
        postalAddress: '',
        timezone: 'UTC',
        accountLinkAvailable: false,
        permissions: [],
        mcpUrl: 'https://sphericon.test',
        trackerUrl: 'https://t.sphericon.test',
        holds: [],
        role: 'owner',
        createdAt: '2026-01-01T00:00:00Z',
      })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)
  await screen.getByRole('button').click()

  await expect.element(screen.getByText('Deleted workspaces')).toBeInTheDocument()
  await expect.element(screen.getByText(/erased .*2026/)).toBeInTheDocument()
  await screen.getByRole('menuitem', { name: 'Restore Initech' }).click()

  await expect.poll(() => restored).toEqual(['InitcWs3'])
  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([{ to: '/workspaces/$workspaceId', params: { workspaceId: 'InitcWs3' } }])
})

test('shows no deleted-workspaces section when none is pending', async () => {
  const { screen } = await renderWithRouter(<UserMenu workspaceId="AcmeWs01" />)
  await screen.getByRole('button', { expanded: false }).click()

  await expect.element(screen.getByText('Sign out')).toBeInTheDocument()
  expect(screen.container.textContent).not.toContain('Deleted workspaces')
})
