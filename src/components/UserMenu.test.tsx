import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import { handleSiteAuthLogout, handleSiteWorkspacesAccountLink } from '../generated/site/msw.gen.ts'
import { profileRoute, settingsRoute } from '../router.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { UserMenu } from './UserMenu.tsx'

test('logout calls the site logout operation and navigates to login', async () => {
  let calls = 0
  worker.use(
    handleSiteAuthLogout(() => {
      calls += 1
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<UserMenu slug="acme" />)

  await screen.getByRole('button').click()
  await screen.getByText('Sign out').click()

  await expect.poll(() => calls).toBe(1)
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/login' }])
})

test('shows workspace settings only when a workspace is active', async () => {
  const { screen } = await renderWithRouter(<UserMenu />)
  await screen.getByRole('button').click()

  await expect.element(screen.getByText('Profile')).toBeInTheDocument()
  expect(screen.container.textContent).not.toContain('Workspace settings')
})

test('profile item navigates to the profile page', async () => {
  const { screen, navigate } = await renderWithRouter(<UserMenu slug="acme" />)
  await screen.getByRole('button').click()
  await screen.getByText('Profile').click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: profileRoute.to }])
})

test('workspace settings item navigates with the active slug', async () => {
  const { screen, navigate } = await renderWithRouter(<UserMenu slug="acme" />)
  await screen.getByRole('button').click()
  await screen.getByText('Workspace settings').click()

  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([{ to: settingsRoute.to, params: { slug: 'acme' } }])
})

test('a failed logout shows an error and stays put', async () => {
  worker.use(handleSiteAuthLogout(() => HttpResponse.error()))
  const { screen, navigate } = await renderWithRouter(<UserMenu slug="acme" />)
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
  const { screen } = await renderWithRouter(<UserMenu slug="acme" />)
  await screen.getByRole('button', { expanded: false }).click()

  await expect.element(screen.getByText('Profile')).toBeInTheDocument()
  expect(screen.container.textContent).not.toContain('Account')
})

test('a failed account link shows an error and opens nothing', async () => {
  worker.use(handleSiteWorkspacesAccountLink(() => HttpResponse.error()))
  const open = vi.spyOn(window, 'open').mockReturnValue(null)
  const { screen } = await renderWithRouter(<UserMenu slug="acme" accountLink />)
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
  const { screen } = await renderWithRouter(<UserMenu slug="acme" accountLink />)
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
