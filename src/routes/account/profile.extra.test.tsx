import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteUserEmailChange,
  handleSiteUserGetMe,
  handleSiteUserSignOutEverywhere,
  handleSiteUserUpdateMe,
} from '../../generated/site/msw.gen.ts'
import type { SiteUserResource } from '../../generated/site/types.gen.ts'
import { loginRoute } from '../../router.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ProfilePage } from './profile.tsx'

const user: SiteUserResource = {
  id: '1',
  name: 'John',
  email: 'info@getsphericon.com',
  emailVerified: false,
  createdAt: '2026-01-01T00:00:00Z',
}

const noContent = () => new Response(null, { status: 204 })

type Op = 'get' | 'update' | 'emailChange' | 'signOutEverywhere'
type Call = { op: Op; body: unknown }
type Handler = (req: Request) => Response | Promise<Response>

// Serves the profile endpoints and records every mutating call.
function serveProfile(
  calls: Call[],
  overrides: Partial<Record<Op, Handler>> = {},
  me: SiteUserResource = user,
) {
  const serve =
    (op: Op, fallback: Handler): Handler =>
    async (req) => {
      if (req.method !== 'GET') {
        const text = await req.clone().text()
        calls.push({ op, body: text ? JSON.parse(text) : null })
      }
      return (overrides[op] ?? fallback)(req)
    }
  const json = () => HttpResponse.json(me)
  worker.use(
    handleSiteUserGetMe(({ request }) => serve('get', json)(request)),
    handleSiteUserUpdateMe(({ request }) => serve('update', json)(request)),
    handleSiteUserEmailChange(({ request }) => serve('emailChange', noContent)(request)),
    handleSiteUserSignOutEverywhere(({ request }) =>
      serve('signOutEverywhere', noContent)(request),
    ),
  )
}

test('changing the password sends the current and new password', async () => {
  const calls: Call[] = []
  serveProfile(calls)
  const { screen } = await renderWithRouter(<ProfilePage />)

  await expect.element(screen.getByLabelText(/^Name/)).toHaveValue('John')
  await screen.getByLabelText('Current password', { exact: true }).first().fill('old-pass')
  await screen.getByLabelText(/^New password/).fill('new-pass-123')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Profile updated')).toBeInTheDocument()
  expect(calls.find((c) => c.op === 'update')?.body).toEqual({
    name: 'John',
    currentPassword: 'old-pass',
    newPassword: 'new-pass-123',
  })
})

test('reports a failed profile update', async () => {
  serveProfile([], {
    update: () => problem(400, { detail: 'wrong password' }),
  })
  const { screen } = await renderWithRouter(<ProfilePage />)

  await expect.element(screen.getByLabelText(/^Name/)).toHaveValue('John')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Failed to update profile')).toBeInTheDocument()
})

test('requests an email change and clears the form', async () => {
  const calls: Call[] = []
  serveProfile(calls)
  const { screen } = await renderWithRouter(<ProfilePage />)

  await screen.getByLabelText(/^New email/).fill(' next@getsphericon.com ')
  await screen
    .getByLabelText(/^Current password/)
    .last()
    .fill('pw-123456')
  await screen.getByRole('button', { name: 'Send confirmation link' }).click()

  await expect
    .element(screen.getByText('Check your new inbox for a confirmation link.'))
    .toBeInTheDocument()
  expect(calls.find((c) => c.op === 'emailChange')?.body).toEqual({
    newEmail: 'next@getsphericon.com',
    currentPassword: 'pw-123456',
  })
  await expect.element(screen.getByLabelText(/^New email/)).toHaveValue('')
})

test('reports a failed email change', async () => {
  serveProfile([], {
    emailChange: () => problem(409, { detail: 'taken' }),
  })
  const { screen } = await renderWithRouter(<ProfilePage />)

  await screen.getByLabelText(/^New email/).fill('next@getsphericon.com')
  await screen
    .getByLabelText(/^Current password/)
    .last()
    .fill('pw-123456')
  await screen.getByRole('button', { name: 'Send confirmation link' }).click()

  await expect.element(screen.getByText('Could not change email')).toBeInTheDocument()
})

test('shows an error when the profile fails to load', async () => {
  worker.use(handleSiteUserGetMe(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<ProfilePage />)

  await expect.element(screen.getByText('Failed to load profile').first()).toBeInTheDocument()
})

test('signing out everywhere asks first, then ends the sessions and goes to login', async () => {
  const calls: Call[] = []
  serveProfile(calls)
  const { screen, navigate } = await renderWithRouter(<ProfilePage />)

  await screen.getByRole('button', { name: 'Sign out everywhere' }).click()
  expect(calls).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Sign out everywhere' }).click()

  await expect.poll(() => calls.map((c) => c.op)).toEqual(['signOutEverywhere'])
  await expect.poll(() => navigate).toHaveBeenCalledWith({ to: loginRoute.to })
})

test('reports a failed sign out everywhere and stays put', async () => {
  serveProfile([], {
    signOutEverywhere: () => problem(500, { detail: 'boom' }),
  })
  const { screen, navigate } = await renderWithRouter(<ProfilePage />)

  await screen.getByRole('button', { name: 'Sign out everywhere' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Sign out everywhere' }).click()

  await expect.element(screen.getByText('Could not sign out everywhere')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalledWith({ to: loginRoute.to })
})
