import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAuthForgotPassword } from '../../generated/site/msw.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ForgotPasswordPage } from './forgot-password.tsx'

test('shows the confirmation state after submitting', async () => {
  // The API returns 202 whether or not the address exists.
  worker.use(handleSiteAuthForgotPassword(() => HttpResponse.json({}, { status: 202 })))
  const { screen } = await renderWithRouter(<ForgotPasswordPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByRole('button', { name: 'Send reset link' }).click()

  await expect.element(screen.getByText('Check your email')).toBeInTheDocument()
})

test('tells how many minutes to wait when the 429 carries a retry time', async () => {
  worker.use(handleSiteAuthForgotPassword(() => problem(429, { retryAfter: 3600 })))
  const { screen } = await renderWithRouter(<ForgotPasswordPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByRole('button', { name: 'Send reset link' }).click()

  await expect.element(screen.getByText(/Try again in 60 minutes/)).toBeInTheDocument()
})

test('tells how many seconds to wait below a minute', async () => {
  worker.use(handleSiteAuthForgotPassword(() => problem(429, { retryAfter: 45 })))
  const { screen } = await renderWithRouter(<ForgotPasswordPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByRole('button', { name: 'Send reset link' }).click()

  await expect.element(screen.getByText(/Try again in 45 seconds/)).toBeInTheDocument()
})

test('falls back to the generic message when the 429 carries no retry time', async () => {
  worker.use(handleSiteAuthForgotPassword(() => problem(429, {})))
  const { screen } = await renderWithRouter(<ForgotPasswordPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByRole('button', { name: 'Send reset link' }).click()

  await expect.element(screen.getByText(/Too many requests/)).toBeInTheDocument()
})
