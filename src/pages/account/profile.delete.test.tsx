import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteSecondFactorGetStatus,
  handleSiteUserDeleteUser,
  handleSiteUserDeleteUserLink,
  handleSiteUserGetMe,
  handleSiteWorkspacesListDeleted,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteDeletedWorkspaceResource,
  SiteSecondFactorStatus,
  SiteUserResource,
} from '../../generated/site/types.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ProfilePage } from './profile.tsx'

const user: SiteUserResource = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  createdAt: '2026-01-01T00:00:00Z',
}

const status = (over: Partial<SiteSecondFactorStatus> = {}): SiteSecondFactorStatus => ({
  enabled: false,
  pending: false,
  recoveryCodesRemaining: 0,
  passwordSet: true,
  providerLoginFresh: false,
  ...over,
})

const waiting: SiteDeletedWorkspaceResource = {
  id: 'BluthW12',
  name: 'Bluth',
  discardedAt: '2026-01-01T00:00:00Z',
  eraseAt: '2026-01-31T00:00:00Z',
}

// Serves the profile page's reads and records every body the deletion receives.
function serve(
  bodies: unknown[],
  over: Partial<SiteSecondFactorStatus> = {},
  deleted: SiteDeletedWorkspaceResource[] = [],
) {
  worker.use(
    handleSiteUserGetMe(() => HttpResponse.json(user)),
    handleSiteSecondFactorGetStatus(() => HttpResponse.json(status(over))),
    handleSiteWorkspacesListDeleted(() => HttpResponse.json(deleted)),
  )
  return (response: () => Response) =>
    worker.use(
      handleSiteUserDeleteUser(async ({ request }) => {
        bodies.push(await request.clone().json())
        return response()
      }),
    )
}

const noContent = () => new Response(null, { status: 204 })

test('deleting the user sends the typed email and password, then lands on the login screen', async () => {
  const bodies: unknown[] = []
  serve(bodies)(noContent)
  const { screen, navigate } = await renderWithRouter(<ProfilePage />)

  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByLabelText(/^Password to confirm/).fill('secret-pass')
  await screen.getByRole('button', { name: 'Delete my user' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ email: 'info@getsphericon.com', currentPassword: 'secret-pass' }])
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: loginRoute.to })
})

test('a user with a second factor also sends a code', async () => {
  const bodies: unknown[] = []
  serve(bodies, { enabled: true })(noContent)
  const { screen } = await renderWithRouter(<ProfilePage />)

  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByLabelText(/^Password to confirm/).fill('secret-pass')
  await screen.getByLabelText(/^Second factor code/).fill('123456')
  await screen.getByRole('button', { name: 'Delete my user' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ email: 'info@getsphericon.com', currentPassword: 'secret-pass', code: '123456' }])
})

test('a workspace waiting for erasure is named with the no-restore warning', async () => {
  serve([], {}, [waiting])(noContent)
  const { screen } = await renderWithRouter(<ProfilePage />)

  await expect
    .element(screen.getByText(/can no longer be restored once you are gone: Bluth/))
    .toBeInTheDocument()
})

test('a refusal for a sole owned workspace names the workspaces and stays put', async () => {
  serve([])(() =>
    problem(422, {
      detail: 'only owner',
      workspaces: [{ id: 'PiperW10', name: 'Pied Piper' }],
    }),
  )
  const { screen, navigate } = await renderWithRouter(<ProfilePage />)

  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByLabelText(/^Password to confirm/).fill('secret-pass')
  await screen.getByRole('button', { name: 'Delete my user' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()

  await expect.element(screen.getByText(/only owner of: Pied Piper/)).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalledWith({ to: loginRoute.to })
})

test('a user with no password asks for the mailed link with their typed email', async () => {
  const bodies: unknown[] = []
  serve([], { passwordSet: false })(noContent)
  worker.use(
    handleSiteUserDeleteUserLink(async ({ request }) => {
      bodies.push(await request.clone().json())
      return new Response(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<ProfilePage />)

  await expect.element(screen.getByLabelText(/^Password to confirm/)).not.toBeInTheDocument()
  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByRole('button', { name: 'Email me a link' }).click()

  await expect.poll(() => bodies).toEqual([{ email: 'info@getsphericon.com' }])
})
