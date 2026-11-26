import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAuthSigninProviders,
  handleSiteIdentitiesLink,
  handleSiteIdentitiesList,
  handleSiteIdentitiesUnlink,
  handleSitePasskeysList,
  handleSiteSecondFactorGetStatus,
} from '../../generated/site/msw.gen.ts'
import type { SiteLinkedIdentities } from '../../generated/site/types.gen.ts'
import { Route as securityRoute } from '../../routes/account/security.tsx'
import { testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SecurityPage } from './security.tsx'

const SECURITY = routeMount(securityRoute)

const GOOGLE = { items: [{ name: 'google', displayName: 'Google' }] }
// The proof a User with a password gives for a link.
const WITH_PASSWORD = { passwordSet: true, identityProofFresh: false }

const LINKED: SiteLinkedIdentities = {
  items: [
    { id: testId(7), provider: 'google', displayName: 'Google', createdAt: '2026-01-01T00:00:00Z' },
  ],
  ...WITH_PASSWORD,
}

const NONE_LINKED: SiteLinkedIdentities = { items: [], ...WITH_PASSWORD }

function serve(identities: SiteLinkedIdentities) {
  worker.use(
    handleSiteSecondFactorGetStatus({
      body: {
        enabled: false,
        pending: false,
        recoveryCodesRemaining: 0,
        passwordSet: true,
        identityProofFresh: false,
      },
    }),
    handleSiteAuthSigninProviders({ body: GOOGLE }),
    handleSiteIdentitiesList({ body: identities }),
  )
}

test('a linked provider offers only Disconnect', async () => {
  serve(LINKED)
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await expect.element(screen.getByRole('button', { name: 'Disconnect' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Connect' }).elements()).toHaveLength(0)
})

test('a provider not linked yet offers only Connect', async () => {
  serve(NONE_LINKED)
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await expect.element(screen.getByRole('button', { name: 'Connect' })).toBeVisible()
  expect(screen.getByRole('button', { name: 'Disconnect' }).elements()).toHaveLength(0)
})

test('connecting a provider asks for the password and starts the link with it', async () => {
  serve(NONE_LINKED)
  const sent: unknown[] = []
  worker.use(
    handleSiteIdentitiesLink(async ({ request }) => {
      sent.push(await request.json())
      // Stay on the page: the test only inspects the link that was started.
      return HttpResponse.json({ redirectUrl: '#done' })
    }),
  )
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await screen.getByRole('button', { name: 'Connect' }).click()
  await screen.getByLabelText(/^Current password/).fill('secret-password')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect
    .poll(() => sent)
    .toEqual([{ provider: 'google', currentPassword: 'secret-password' }])
})

test('a User with no password and a fresh proof connects without typing one', async () => {
  serve({ items: [], passwordSet: false, identityProofFresh: true })
  const sent: unknown[] = []
  worker.use(
    handleSiteIdentitiesLink(async ({ request }) => {
      sent.push(await request.json())
      return HttpResponse.json({ redirectUrl: '#done' })
    }),
  )
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await screen.getByRole('button', { name: 'Connect' }).click()
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.poll(() => sent).toEqual([{ provider: 'google' }])
})

test('a User with no password whose proof is stale is sent to a fresh check first', async () => {
  serve({ items: [], passwordSet: false, identityProofFresh: false })
  worker.use(
    handleSitePasskeysList({
      body: { items: [], available: true, passwordSet: false, identityProofFresh: false },
    }),
  )
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await screen.getByRole('button', { name: 'Connect' }).click()

  await expect
    .element(screen.getByText(/connecting a sign-in provider needs a fresh check/))
    .toBeVisible()
  expect(screen.getByRole('button', { name: 'Continue' }).elements()).toHaveLength(0)
})

test('a link the server refuses is reported', async () => {
  serve(NONE_LINKED)
  worker.use(handleSiteIdentitiesLink(() => problem(403, 'the current password is incorrect')))
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await screen.getByRole('button', { name: 'Connect' }).click()
  await screen.getByLabelText(/^Current password/).fill('wrong')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.element(screen.getByText('Could not connect the account')).toBeVisible()
})

test('an identity of a provider the Instance no longer offers can still be disconnected', async () => {
  serve({
    items: [
      {
        id: testId(8),
        provider: 'github',
        displayName: 'github',
        createdAt: '2026-01-01T00:00:00Z',
      },
    ],
    ...WITH_PASSWORD,
  })
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await expect.element(screen.getByText('github')).toBeVisible()
  await expect.element(screen.getByRole('button', { name: 'Disconnect' })).toBeVisible()
  await expect.element(screen.getByRole('button', { name: 'Connect' })).toBeVisible()
})

test('unlinking a provider calls the unlink operation of that identity', async () => {
  serve(LINKED)
  const called: string[] = []
  worker.use(
    handleSiteIdentitiesUnlink(({ params }) => {
      called.push(params['id'])
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await screen.getByRole('button', { name: 'Disconnect' }).click()

  await expect.poll(() => called).toEqual([testId(7)])
})

test('an unlink the server refuses is reported', async () => {
  serve(LINKED)
  worker.use(
    handleSiteIdentitiesUnlink(() =>
      problem(
        409,
        'this is your only way to sign in: set a password or link another provider first',
      ),
    ),
  )
  const { screen } = await renderWithRouter(<SecurityPage />, SECURITY)

  await screen.getByRole('button', { name: 'Disconnect' }).click()

  await expect.element(screen.getByText('Could not disconnect the provider')).toBeVisible()
})

test('a refused link is explained when the callback lands on the settings', async () => {
  serve(LINKED)
  const { screen } = await renderWithRouter(<SecurityPage />, {
    ...SECURITY,
    initialPath: `${SECURITY.initialPath}?error=identity_in_use`,
  })

  await expect.element(screen.getByText('Could not connect the account')).toBeVisible()
})
