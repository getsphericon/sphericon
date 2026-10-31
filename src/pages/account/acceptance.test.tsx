import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAuthLogout,
  handleSiteLegalDocumentsGet,
  handleSiteSecondFactorGetStatus,
  handleSiteUserAcceptPending,
  handleSiteUserDeleteUser,
  handleSiteUserGetMe,
  handleSiteWorkspacesDelete,
  handleSiteWorkspacesListDeleted,
} from '../../generated/site/msw.gen.ts'
import type { SiteUserResource } from '../../generated/site/types.gen.ts'
import { Route as acceptanceRoute } from '../../routes/acceptance.tsx'
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AcceptancePage } from './acceptance.tsx'

const mount = routeMount(acceptanceRoute)

// A User the new version of the terms stops (ADR 0062).
const gated: SiteUserResource = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: true,
  timezone: null,
  signupSurveyDue: false,
  acceptancesPending: [
    {
      key: 'service-terms',
      title: 'Terms of service',
      kind: 'required_acceptance',
      version: '2026-02',
    },
  ],
  createdAt: '2026-01-01T00:00:00Z',
}

function serveScreen() {
  worker.use(
    handleSiteUserGetMe(() => HttpResponse.json(gated)),
    handleSiteLegalDocumentsGet(() =>
      HttpResponse.json({
        key: 'service-terms',
        title: 'Terms of service',
        version: '2026-02',
        markdown: '## What changed\n\nWe now keep your **address**.',
        publishedAt: '2026-02-01T00:00:00Z',
      }),
    ),
    handleSiteSecondFactorGetStatus(() =>
      HttpResponse.json({
        enabled: false,
        pending: false,
        recoveryCodesRemaining: 0,
        passwordSet: true,
        identityProofFresh: false,
      }),
    ),
    handleSiteWorkspacesListDeleted(() => HttpResponse.json([])),
  )
}

test('the screen shows the new text and accepting sends the versions shown', async () => {
  serveScreen()
  const bodies: unknown[] = []
  worker.use(
    handleSiteUserAcceptPending(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ ...gated, acceptancesPending: [] })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptancePage />, mount)

  await expect
    .element(screen.getByRole('heading', { name: 'Terms of service' }))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('heading', { name: 'What changed' })).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Accept and continue' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ items: [{ key: 'service-terms', version: '2026-02' }] }])
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: indexRoute.to })
})

test('accepting resumes the page the gate stopped the user on', async () => {
  serveScreen()
  worker.use(handleSiteUserAcceptPending({ body: { ...gated, acceptancesPending: [] } }))
  const consent = '/oauth/consent?client_id=c&state=s'
  const { screen, navigate } = await renderWithRouter(<AcceptancePage />, {
    ...mount,
    initialPath: `${mount.initialPath}?${new URLSearchParams({ redirect: consent }).toString()}`,
  })

  await screen.getByRole('button', { name: 'Accept and continue' }).click()

  await expect.poll(() => navigate).toHaveBeenCalledWith({ href: consent })
})

test('signing out from the screen lands on the login', async () => {
  serveScreen()
  let signedOut = false
  worker.use(
    handleSiteAuthLogout(() => {
      signedOut = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptancePage />, mount)

  await screen.getByRole('button', { name: 'Sign out' }).click()

  await expect.poll(() => signedOut).toBe(true)
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: loginRoute.to })
})

test('a gated user deletes their account from the screen', async () => {
  serveScreen()
  const bodies: unknown[] = []
  worker.use(
    handleSiteUserDeleteUser(async ({ request }) => {
      bodies.push(await request.json())
      return new Response(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptancePage />, mount)

  await screen.getByRole('button', { name: 'Delete my user' }).click()
  await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
  await screen.getByLabelText(/^Password to confirm/).fill('secret-pass')
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([{ email: 'info@getsphericon.com', currentPassword: 'secret-pass' }])
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: loginRoute.to })
})

test('a gated sole owner deletes the workspace the refused deletion names, then their account', async () => {
  serveScreen()
  const workspaceBodies: unknown[] = []
  worker.use(
    handleSiteUserDeleteUser(() =>
      problem(422, {
        detail: 'You are the only owner of Pied Piper.',
        workspaces: [{ id: 'PiperW10', name: 'Pied Piper' }],
      }),
    ),
    handleSiteWorkspacesDelete(async ({ request }) => {
      workspaceBodies.push(await request.json())
      return HttpResponse.json({
        id: 'PiperW10',
        name: 'Pied Piper',
        discardedAt: '2026-02-01T00:00:00Z',
        eraseAt: '2026-03-03T00:00:00Z',
        onlyOwner: true,
      })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<AcceptancePage />, mount)
  // The modal opens with an empty form each time: a refusal closes it so the page offers the
  // Workspaces it names.
  const deleteAccount = async () => {
    await screen.getByRole('button', { name: 'Delete my user' }).click()
    await screen.getByLabelText(/^Type your email/).fill('info@getsphericon.com')
    await screen.getByLabelText(/^Password to confirm/).fill('secret-pass')
    await screen.getByRole('dialog').getByRole('button', { name: 'Delete my user' }).click()
  }

  await deleteAccount()
  await screen.getByRole('button', { name: 'Delete Pied Piper' }).click()
  await screen.getByLabelText(/^Type Pied Piper to confirm/).fill('Pied Piper')
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete workspace' }).click()

  await expect.poll(() => workspaceBodies).toEqual([{ name: 'Pied Piper' }])
  await expect
    .element(screen.getByRole('button', { name: 'Delete Pied Piper' }))
    .not.toBeInTheDocument()
  worker.use(handleSiteUserDeleteUser(() => new Response(null, { status: 204 })))
  await deleteAccount()
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: loginRoute.to })
})
