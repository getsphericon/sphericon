import { notifications } from '@mantine/notifications'
import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import {
  handleOperatorAuthLogin,
  handleOperatorAuthSecondFactor,
} from '../generated/operator/msw.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { ConsoleLoginPage } from './login.tsx'

afterEach(() => {
  notifications.clean()
})

const operator = { id: '1', email: 'ops@example.com' }

test('a TOTP challenge asks for a code and opens the console', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleOperatorAuthLogin({ body: { outcome: 'challenge', challenge: 'signed' } }),
    handleOperatorAuthSecondFactor(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(operator)
    }),
  )
  const { screen, navigate } = await renderWithRouter(<ConsoleLoginPage />)

  await screen.getByLabelText(/^Email/).fill('ops@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()
  await screen.getByLabelText(/^Authentication code/).fill('123456')
  expect(navigate).not.toHaveBeenCalled()
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(bodies).toEqual([{ challenge: 'signed', code: '123456' }])
})

test('first login shows the enrolment secret and confirms it with the first code', async () => {
  worker.use(
    handleOperatorAuthLogin({
      body: {
        outcome: 'enrolment',
        challenge: 'signed',
        enrolment: {
          secret: 'JBSWY3DPEHPK3PXP',
          uri: 'otpauth://totp/x',
          qrCode: 'data:image/png;base64,AAAA',
        },
      },
    }),
    handleOperatorAuthSecondFactor({ body: operator }),
  )
  const { screen, navigate } = await renderWithRouter(<ConsoleLoginPage />)

  await screen.getByLabelText(/^Email/).fill('ops@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText('JBSWY3DPEHPK3PXP')).toBeInTheDocument()
  await screen.getByLabelText(/^Authentication code/).fill('654321')
  await screen.getByRole('button', { name: 'Confirm and sign in' }).click()

  await expect.poll(() => navigate.mock.calls.length).toBe(1)
})

test('a wrong password shows the error and stays on the password step', async () => {
  worker.use(handleOperatorAuthLogin(() => problem(401, { detail: 'Invalid credentials' })))
  const { screen, navigate } = await renderWithRouter(<ConsoleLoginPage />)

  await screen.getByLabelText(/^Email/).fill('ops@example.com')
  await screen.getByLabelText(/^Password/).fill('wrong')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText('Invalid credentials')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('a rate-limited attempt says how long to wait', async () => {
  worker.use(handleOperatorAuthLogin(() => problem(429, { retryAfter: 90 })))
  const { screen } = await renderWithRouter(<ConsoleLoginPage />)

  await screen.getByLabelText(/^Email/).fill('ops@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()

  await expect.element(screen.getByText(/Try again in 2 minutes/)).toBeInTheDocument()
})
