import { notifications } from '@mantine/notifications'
import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import { handleSiteAuthVerifyEmail } from '../../generated/site/msw.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as verifyEmailRoute } from '../../routes/verify-email.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { VerifyEmailPage } from './verify-email.tsx'

const mount = routeMount(verifyEmailRoute)
const MOUNT = { ...mount, initialPath: `${mount.initialPath}?token=tok-v` }

// Toasts live in a global store and would cover the form's buttons in the next test.
afterEach(() => {
  notifications.clean()
})

async function fillAndSubmit(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  confirm: string,
) {
  await screen.getByLabelText(/^Password/).fill('s3cret-pass')
  await screen.getByLabelText(/^Confirm password/).fill(confirm)
  await screen.getByRole('button', { name: 'Set password' }).click()
}

test('sends the token with the chosen password and offers to sign in', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthVerifyEmail(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await fillAndSubmit(screen, 's3cret-pass')

  await expect.element(screen.getByText('Email verified')).toBeInTheDocument()
  expect(bodies).toEqual([{ token: 'tok-v', password: 's3cret-pass' }])
  await expect
    .element(screen.getByRole('link', { name: 'Sign in' }))
    .toHaveAttribute('href', loginRoute.to)
})

test('does not send anything when the confirmation differs', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthVerifyEmail(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await fillAndSubmit(screen, 'something-else')

  await expect.element(screen.getByText('Passwords do not match')).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test('shows the failure and keeps the form when the link is rejected', async () => {
  worker.use(
    handleSiteAuthVerifyEmail(() =>
      problem(400, { detail: 'invalid or expired verification link' }),
    ),
  )
  const { screen } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await fillAndSubmit(screen, 's3cret-pass')

  await expect.element(screen.getByText('Verification failed')).toBeInTheDocument()
  await expect.element(screen.getByText('invalid or expired verification link')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Set password' })).toBeInTheDocument()
})
