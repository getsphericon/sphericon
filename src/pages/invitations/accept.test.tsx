import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSitePublicInvitationsAccept,
  handleSitePublicInvitationsLookup,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteInvitationLookupResult } from '../../generated/site/types.gen.ts'
import { Route as acceptInvitationRoute } from '../../routes/invitations/$token.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AcceptInvitationPage } from './accept.tsx'

const MOUNT = routeMount(acceptInvitationRoute, { token: 'inv-1' })

const invite: SiteInvitationLookupResult = { workspaceName: 'Acme', email: 'ada@example.com' }

test('a signed-out invitee supplies a name and password to accept', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList(() => problem(401)),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSitePublicInvitationsAccept(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByText("You've been invited to join Acme on sphericon."))
    .toBeInTheDocument()
  await expect.element(screen.getByText('ada@example.com')).toBeInTheDocument()
  await screen.getByLabelText(/^Your name/).fill('  Ada  ')
  await screen.getByLabelText(/^Password/).fill('s3cret')
  await screen.getByRole('button', { name: 'Accept invitation' }).click()

  await expect.element(screen.getByText('Invitation accepted')).toBeInTheDocument()
  expect(bodies).toEqual([{ name: 'Ada', password: 's3cret' }])
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: loginRoute.to }])
})

test('a signed-in invitee can join with an empty body', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSitePublicInvitationsAccept(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await screen.getByRole('button', { name: 'Join workspace' }).click()

  await expect.element(screen.getByText('Invitation accepted')).toBeInTheDocument()
  await expect.poll(() => bodies).toEqual([{}])
})

test('a signed-out invitee with an account is sent to sign in and back to the accept page', async () => {
  worker.use(
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSiteWorkspacesList(() => problem(401)),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByRole('button', { name: 'Join workspace' }))
    .not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect
    .poll(() => navigate.mock.calls)
    .toContainEqual([{ to: loginRoute.to, search: { redirect: expect.stringContaining('inv-1') } }])
})

test('an accept failure shows the API error and stays on the page', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [] }),
    handleSitePublicInvitationsLookup({ body: invite }),
    handleSitePublicInvitationsAccept(() => problem(410, { detail: 'Invitation revoked' })),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await screen.getByRole('button', { name: 'Join workspace' }).click()

  await expect.element(screen.getByText('Invitation revoked')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('shows a loading state while the invitation is looked up', async () => {
  worker.use(handleSitePublicInvitationsLookup(() => new Promise<Response>(() => {})))
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect.element(screen.getByText('Loading invitation…')).toBeInTheDocument()
})

test('an unknown invitation shows the invalid alert', async () => {
  worker.use(handleSitePublicInvitationsLookup(() => problem(404)))
  const { screen } = await renderWithRouter(<AcceptInvitationPage />, MOUNT)

  await expect
    .element(screen.getByText('This invitation is invalid or has expired.'))
    .toBeInTheDocument()
})
