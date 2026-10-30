import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAuthSigninProviders,
  handleSiteIdentitiesList,
  handleSiteIdentitiesUnlink,
  handleSiteSecondFactorGetStatus,
} from '../../generated/site/msw.gen.ts'
import type { SiteLinkedIdentity } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { SecurityPage } from './security.tsx'

const GOOGLE = { items: [{ name: 'google', displayName: 'Google' }] }
const LINKED = {
  items: [
    { id: '7', provider: 'google', displayName: 'Google', createdAt: '2026-01-01T00:00:00Z' },
  ],
}

function serve(identities: { items: SiteLinkedIdentity[] }) {
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
  const { screen } = await renderWithRouter(<SecurityPage />)

  await expect.element(screen.getByRole('button', { name: 'Disconnect' })).toBeVisible()
  expect(screen.getByRole('link', { name: 'Connect' }).elements()).toHaveLength(0)
})

test('a provider not linked yet offers only Connect', async () => {
  serve({ items: [] })
  const { screen } = await renderWithRouter(<SecurityPage />)

  await expect
    .element(screen.getByRole('link', { name: 'Connect' }))
    .toHaveAttribute('href', '/site/me/identities/google/link')
  expect(screen.getByRole('button', { name: 'Disconnect' }).elements()).toHaveLength(0)
})

test('an identity of a provider the Instance no longer offers can still be disconnected', async () => {
  serve({
    items: [
      { id: '8', provider: 'github', displayName: 'github', createdAt: '2026-01-01T00:00:00Z' },
    ],
  })
  const { screen } = await renderWithRouter(<SecurityPage />)

  await expect.element(screen.getByText('github')).toBeVisible()
  await expect.element(screen.getByRole('button', { name: 'Disconnect' })).toBeVisible()
  await expect.element(screen.getByRole('link', { name: 'Connect' })).toBeVisible()
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
  const { screen } = await renderWithRouter(<SecurityPage />)

  await screen.getByRole('button', { name: 'Disconnect' }).click()

  await expect.poll(() => called).toEqual(['7'])
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
  const { screen } = await renderWithRouter(<SecurityPage />)

  await screen.getByRole('button', { name: 'Disconnect' }).click()

  await expect.element(screen.getByText('Could not disconnect the provider')).toBeVisible()
})

test('a refused link is explained when the callback lands on the settings', async () => {
  serve(LINKED)
  const { screen } = await renderWithRouter(<SecurityPage />, {
    initialPath: '/workspaces/TestWs01?error=identity_in_use',
  })

  await expect.element(screen.getByText('Could not connect the account')).toBeVisible()
})
