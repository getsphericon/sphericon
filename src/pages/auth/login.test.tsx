import type { AuthenticationResponseJSON } from '@simplewebauthn/browser'
import { HttpResponse } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'

import {
  handleSiteAuthLogin,
  handleSiteAuthResendVerification,
  handleSiteAuthSecondFactor,
  handleSitePasskeyLoginBegin,
  handleSitePasskeyLoginFinish,
} from '../../generated/site/msw.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { LoginPage } from './login.tsx'

// The authenticator is the browser's: tests stand in for it, with the ceremony options and the
// answer it would give.
const webauthn = vi.hoisted(() => ({
  supported: vi.fn<() => boolean>(),
  autofill: vi.fn<() => Promise<boolean>>(),
  authenticate: vi.fn<(args: unknown) => Promise<AuthenticationResponseJSON>>(),
  cancel: vi.fn<() => void>(),
}))
vi.mock('@simplewebauthn/browser', () => ({
  browserSupportsWebAuthn: webauthn.supported,
  browserSupportsWebAuthnAutofill: webauthn.autofill,
  startAuthentication: webauthn.authenticate,
  WebAuthnAbortService: { cancelCeremony: webauthn.cancel },
}))

const OPTIONS = {
  challenge: 'Y2hhbGxlbmdl',
  rpId: 'sphericon.test',
  userVerification: 'required' as const,
}
const ASSERTION: AuthenticationResponseJSON = {
  id: 'abc',
  rawId: 'abc',
  type: 'public-key',
  clientExtensionResults: {},
  response: { clientDataJSON: 'e30', authenticatorData: 'e30', signature: 'c2ln' },
}

beforeEach(() => {
  webauthn.supported.mockReturnValue(true)
  webauthn.autofill.mockResolvedValue(false)
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

test('the verify-required refusal offers to send the link again to the typed address', async () => {
  const resendBodies: unknown[] = []
  worker.use(
    handleSiteAuthLogin(() => problem(403, { code: 'email_verification_required' })),
    handleSiteAuthResendVerification(async ({ request }) => {
      resendBodies.push(await request.json())
      return new Response(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<LoginPage />)

  await screen.getByLabelText(/^Email/).fill('user@example.com')
  await screen.getByLabelText(/^Password/).fill('secret')
  await screen.getByRole('button', { name: 'Sign in' }).click()
  await screen.getByRole('button', { name: 'Resend verification email' }).click()

  await expect
    .element(
      screen.getByText('If this address is waiting for verification, we sent the link again'),
    )
    .toBeInTheDocument()
  expect(resendBodies).toEqual([{ email: 'user@example.com' }])
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

test('the Passkey button logs in without an email or a password', async () => {
  webauthn.authenticate.mockResolvedValue(ASSERTION)
  const finished: unknown[] = []
  worker.use(
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish(async ({ request }) => {
      finished.push(await request.json())
      return HttpResponse.json({ outcome: 'session' })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByRole('button', { name: 'Use a passkey' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(webauthn.authenticate).toHaveBeenCalledWith({
    optionsJSON: OPTIONS,
    useBrowserAutofill: false,
  })
  expect(finished).toEqual([{ credential: ASSERTION }])
})

test('the browser offers the Passkeys in the email field and the login finishes on a choice', async () => {
  webauthn.autofill.mockResolvedValue(true)
  webauthn.authenticate.mockResolvedValue(ASSERTION)
  worker.use(
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish({ body: { outcome: 'session' } }),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(webauthn.authenticate).toHaveBeenCalledWith({
    optionsJSON: OPTIONS,
    useBrowserAutofill: true,
  })
  await expect
    .element(screen.getByLabelText(/^Email/))
    .toHaveAttribute('autocomplete', 'username webauthn')
})

test('a refused Passkey login says so and stays on the page', async () => {
  webauthn.authenticate.mockResolvedValue(ASSERTION)
  worker.use(
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish(() => problem(401, { detail: 'the passkey sign-in failed' })),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await screen.getByRole('button', { name: 'Use a passkey' }).click()

  await expect.element(screen.getByText('the passkey sign-in failed')).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('a browser without WebAuthn offers no Passkey button', async () => {
  webauthn.supported.mockReturnValue(false)
  const { screen } = await renderWithRouter(<LoginPage />)

  await expect.element(screen.getByLabelText(/^Email/)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Use a passkey' }).query()).toBeNull()
})
