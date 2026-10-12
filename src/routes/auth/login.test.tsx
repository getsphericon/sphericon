import { notifications } from '@mantine/notifications'
import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import { handleSiteAuthLogin, handleSiteAuthSecondFactor } from '../../generated/site/msw.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { LoginPage } from './login.tsx'

// Toasts live in a global store and would cover the form's buttons in the next test.
afterEach(() => {
  notifications.clean()
})

test('navigates home after a successful login', async () => {
  worker.use(handleSiteAuthLogin({ body: { outcome: 'session' } }))
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  // Required fields render their label as "Password *", hence the anchored regex.
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
})

test('shows an error notification when login fails', async () => {
  worker.use(handleSiteAuthLogin(() => problem(401, { detail: 'Invalid credentials' })))
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  // Required fields render their label as "Password *", hence the anchored regex.
  await screen.getByLabelText(/^Password/).fill('wrong')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText('Invalid credentials')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('tells an unverified user to verify their email', async () => {
  worker.use(
    handleSiteAuthLogin(() =>
      problem(403, { code: 'email_verification_required', detail: 'server detail' }),
    ),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText('Verify your email first')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('shows how long to wait when login is rate limited', async () => {
  worker.use(handleSiteAuthLogin(() => problem(429, { retryAfter: 90 })))
  const { screen } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText(/Try again in 2 minutes/)).toBeInTheDocument()
})

test('shows the wait in seconds when it is under a minute', async () => {
  worker.use(handleSiteAuthLogin(() => problem(429, { retryAfter: 4 })))
  const { screen } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText(/Try again in 4 seconds/)).toBeInTheDocument()
})

test('asks for the second step only when the login answers with a challenge', async () => {
  const secondStepBodies: unknown[] = []
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSiteAuthSecondFactor(async ({ request }) => {
      secondStepBodies.push(await request.json())
      return HttpResponse.json({ outcome: 'session' })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('sam@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await screen.getByLabelText(/^Authentication code/).fill('123456')
  expect(navigate).not.toHaveBeenCalled()
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(secondStepBodies).toEqual([{ challenge: 'signed-challenge', code: '123456' }])
})

test('a session outcome skips the second step', async () => {
  worker.use(handleSiteAuthLogin({ body: { outcome: 'session' } }))
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(screen.getByLabelText(/^Authentication code/).query()).toBeNull()
})

test('shows an error and stays on the second step when the code is wrong', async () => {
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSiteAuthSecondFactor(() => problem(401, { detail: 'the code is not valid' })),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('sam@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()
  await screen.getByLabelText(/^Authentication code/).fill('000000')
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.element(screen.getByText('the code is not valid')).toBeInTheDocument()
  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})
