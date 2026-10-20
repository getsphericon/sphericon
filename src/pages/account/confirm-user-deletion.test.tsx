import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteSecondFactorGetStatus,
  handleSiteUserDeleteUser,
  handleSiteWorkspacesListDeleted,
} from '../../generated/site/msw.gen.ts'
import type { SiteDeletedWorkspaceResource } from '../../generated/site/types.gen.ts'
import { Route as deleteRoute } from '../../routes/account/delete.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ConfirmUserDeletionPage } from './confirm-user-deletion.tsx'

const mount = routeMount(deleteRoute)
const linked = { ...mount, initialPath: `${mount.initialPath}?token=link-token` }

const waiting: SiteDeletedWorkspaceResource = {
  id: 'BluthW12',
  name: 'Bluth',
  discardedAt: '2026-01-01T00:00:00Z',
  eraseAt: '2026-01-31T00:00:00Z',
  onlyOwner: true,
}

function serve(bodies: unknown[], enabled: boolean, deleted: SiteDeletedWorkspaceResource[] = []) {
  worker.use(
    handleSiteSecondFactorGetStatus(() =>
      HttpResponse.json({
        enabled,
        pending: false,
        recoveryCodesRemaining: 0,
        passwordSet: false,
        providerLoginFresh: false,
      }),
    ),
    handleSiteWorkspacesListDeleted(() => HttpResponse.json(deleted)),
    handleSiteUserDeleteUser(async ({ request }) => {
      bodies.push(await request.clone().json())
      return new Response(null, { status: 204 })
    }),
  )
}

test('the link token and the typed email finish the deletion', async () => {
  const bodies: unknown[] = []
  serve(bodies, false)
  const { screen, navigate } = await renderWithRouter(<ConfirmUserDeletionPage />, linked)

  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByRole('button', { name: 'Delete my user' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()

  await expect.poll(() => bodies).toEqual([{ email: 'info@getsphericon.com', token: 'link-token' }])
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: loginRoute.to })
})

test('a user with a second factor also sends a code', async () => {
  const bodies: unknown[] = []
  serve(bodies, true)
  const { screen } = await renderWithRouter(<ConfirmUserDeletionPage />, linked)

  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByLabelText(/^Second factor code/).fill('123456')
  await screen.getByRole('button', { name: 'Delete my user' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ email: 'info@getsphericon.com', token: 'link-token', code: '123456' }])
})

test('only a workspace the user solely owns is named in the no-restore warning', async () => {
  serve([], false, [waiting, { ...waiting, id: 'DundrW13', name: 'Dunder', onlyOwner: false }])
  const { screen } = await renderWithRouter(<ConfirmUserDeletionPage />, linked)

  await expect.element(screen.getByText(/once you are gone: Bluth\./)).toBeInTheDocument()
  await expect.element(screen.getByText(/Dunder/)).not.toBeInTheDocument()
})
