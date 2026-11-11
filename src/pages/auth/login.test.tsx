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
import { settled } from '../../test/queryClients.ts'
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

// The password form, filled and sent.
async function signInWithPassword(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  email = 'sam@example.com',
  password = 'secret',
) {
  await screen.getByLabelText(/^Email/).fill(email)
  // Required fields render their label as "Password *", hence the anchored regex.
  await screen.getByLabelText(/^Password/).fill(password)
  await screen.getByRole('button', { name: 'Sign in' }).click()
}

// A guard sent the person here to sign in, so a signed-in page moves the browser on to `NEXT`:
// where its (real, in-memory) history went is what a test reads, rather than the calls of the
// `navigate` the harness stubs.
const NEXT = '/oauth/consent'
async function renderLogin(search = '') {
  return renderWithRouter(<LoginPage />, {
    initialPath: `/login?redirect=${encodeURIComponent(NEXT)}${search}`,
    path: '/login',
  })
}

// The browser's Passkey ceremony, driven by the test: it starts when the page asks and waits for
// the person, who chooses a Passkey or dismisses the prompt; a cancel from the page ends it with an
// AbortError, as the browser's does.
function passkeyCeremony() {
  const started = Promise.withResolvers<unknown>()
  const answer = Promise.withResolvers<AuthenticationResponseJSON>()
  // An answer settled before the page asked is not an unhandled rejection: the page reads it later.
  answer.promise.catch(() => undefined)
  webauthn.authenticate.mockImplementation(async (args) => {
    started.resolve(args)
    return answer.promise
  })
  webauthn.cancel.mockImplementation(() => {
    answer.reject(new DOMException('cancelled', 'AbortError'))
  })
  return {
    started: started.promise,
    choose: () => answer.resolve(ASSERTION),
    dismiss: () => answer.reject(new DOMException('dismissed', 'NotAllowedError')),
  }
}

beforeEach(() => {
  webauthn.supported.mockReturnValue(true)
  webauthn.autofill.mockResolvedValue(false)
})

test('navigates home after a successful login', async () => {
  worker.use(handleSiteAuthLogin({ body: { outcome: 'session' } }))
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen, 'user@example.com')

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
})

test('shows an error notification when login fails', async () => {
  worker.use(handleSiteAuthLogin(() => problem(401, { detail: 'Invalid credentials' })))
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen, 'user@example.com', 'wrong')

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

  await signInWithPassword(screen, 'user@example.com')

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

  await signInWithPassword(screen, 'user@example.com')
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

  await signInWithPassword(screen, 'user@example.com')

  await expect.element(screen.getByText(/Try again in 2 minutes/)).toBeInTheDocument()
})

test('shows the wait in seconds when it is under a minute', async () => {
  worker.use(handleSiteAuthLogin(() => problem(429, { retryAfter: 4 })))
  const { screen } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen, 'user@example.com')

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

  await signInWithPassword(screen)

  await screen.getByLabelText(/^Authentication code/).fill('123456')
  expect(navigate).not.toHaveBeenCalled()
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(secondStepBodies).toEqual([{ challenge: 'signed-challenge', code: '123456' }])
})

test('the second step takes a recovery code instead of the app code', async () => {
  const secondStepBodies: unknown[] = []
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSiteAuthSecondFactor(async ({ request }) => {
      secondStepBodies.push(await request.json())
      return HttpResponse.json({ outcome: 'session' })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen)
  await screen.getByRole('button', { name: 'Use a recovery code' }).click()
  await screen.getByLabelText(/^Recovery code/).fill('abcde-fghij')
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(secondStepBodies).toEqual([{ challenge: 'signed-challenge', code: 'abcde-fghij' }])
})

test('a session outcome skips the second step', async () => {
  worker.use(handleSiteAuthLogin({ body: { outcome: 'session' } }))
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen, 'user@example.com')

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(screen.getByLabelText(/^Authentication code/).query()).toBeNull()
})

test('shows an error and stays on the second step when the code is wrong', async () => {
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSiteAuthSecondFactor(() => problem(401, { detail: 'the code is not valid' })),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen)
  await screen.getByLabelText(/^Authentication code/).fill('000000')
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.element(screen.getByText('the code is not valid')).toBeInTheDocument()
  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
  expect(navigate).not.toHaveBeenCalled()
})

test('a code step abandoned by Start over fails late without touching the password view', async () => {
  const codeAnswer = Promise.withResolvers<Response>()
  const codeAsked = Promise.withResolvers<Request>()
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSiteAuthSecondFactor(async ({ request }) => {
      codeAsked.resolve(request)
      return codeAnswer.promise
    }),
  )
  const { screen } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen)
  await screen.getByLabelText(/^Authentication code/).fill('000000')
  await screen.getByRole('button', { name: 'Verify' }).click()
  await codeAsked.promise
  await screen.getByRole('button', { name: 'Start over' }).click()
  await screen.getByLabelText(/^Password/).fill('typed-again')
  codeAnswer.resolve(problem(401, { detail: 'the code is not valid' }))

  // The late answer has been handled once no change is running.
  await settled()
  expect(screen.getByText('the code is not valid').query()).toBeNull()
  await expect.element(screen.getByLabelText(/^Email/)).toHaveValue('sam@example.com')
  await expect.element(screen.getByLabelText(/^Password/)).toHaveValue('typed-again')
})

// A challenge is signed over its expiry in whole seconds, so a second login within the same second
// is handed the very same string: the attempt is the code step, not the challenge it carries.
test('a new code step with the same challenge ignores the code sent to the abandoned one', async () => {
  const codeAnswer = Promise.withResolvers<Response>()
  const codeAsked = Promise.withResolvers<Request>()
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSiteAuthSecondFactor(async ({ request }) => {
      codeAsked.resolve(request)
      return codeAnswer.promise
    }),
  )
  const { screen } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen)
  await screen.getByLabelText(/^Authentication code/).fill('000000')
  await screen.getByRole('button', { name: 'Verify' }).click()
  await codeAsked.promise
  await screen.getByRole('button', { name: 'Start over' }).click()
  await signInWithPassword(screen)
  await screen.getByLabelText(/^Authentication code/).fill('123456')

  await expect.element(screen.getByRole('button', { name: 'Verify' })).toBeEnabled()
  codeAnswer.resolve(problem(401, { detail: 'the code is not valid' }))
  await settled()
  expect(screen.getByText('the code is not valid').query()).toBeNull()
  // The code typed on the new step is still in its boxes.
  await expect.element(screen.getByLabelText(/^Authentication code/)).toHaveValue('1')
  await expect.element(screen.getByLabelText('Digit 6')).toHaveValue('6')
})

test('a sign-in whose navigation failed can be tried again', async () => {
  worker.use(handleSiteAuthLogin({ body: { outcome: 'session' } }))
  const { screen, router } = await renderLogin()
  vi.spyOn(router.history, 'push').mockImplementationOnce(() => {
    throw new Error('navigation failed')
  })

  await signInWithPassword(screen)
  await settled()
  await signInWithPassword(screen)

  await expect.poll(() => router.history.location.pathname).toBe(NEXT)
})

test('a Passkey from autofill that signs in after a password login leaves the page once', async () => {
  webauthn.autofill.mockResolvedValue(true)
  const ceremony = passkeyCeremony()
  const finished = Promise.withResolvers<Response>()
  const finishAsked = Promise.withResolvers<Request>()
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'session' } }),
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish(async ({ request }) => {
      finishAsked.resolve(request)
      return finished.promise
    }),
  )
  const { screen, router } = await renderLogin()
  const entries = router.history.length

  await ceremony.started
  ceremony.choose()
  await finishAsked.promise
  await signInWithPassword(screen)
  await expect.poll(() => router.history.location.pathname).toBe(NEXT)
  finished.resolve(HttpResponse.json({ outcome: 'session' }))

  await settled()
  expect(router.history.length).toBe(entries + 1)
})

test('a challenge cancels the Passkey ceremony the browser offers in the email field', async () => {
  webauthn.autofill.mockResolvedValue(true)
  const ceremony = passkeyCeremony()
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish({ body: { outcome: 'session' } }),
  )
  const { screen, router } = await renderLogin()

  await ceremony.started
  await signInWithPassword(screen)
  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
  // The person picks a Passkey from a list that is no longer offered.
  ceremony.choose()

  await settled()
  expect(router.history.location.pathname).toBe('/login')
  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
})

test('a challenge that lands while the autofill ceremony is still being prepared stops it', async () => {
  webauthn.autofill.mockResolvedValue(true)
  // Were the ceremony to start, the person would at once choose a Passkey.
  webauthn.authenticate.mockResolvedValue(ASSERTION)
  const begun = Promise.withResolvers<Response>()
  const beginAsked = Promise.withResolvers<Request>()
  worker.use(
    handleSiteAuthLogin({ body: { outcome: 'challenge', challenge: 'signed-challenge' } }),
    handleSitePasskeyLoginBegin(async ({ request }) => {
      beginAsked.resolve(request)
      return begun.promise
    }),
    handleSitePasskeyLoginFinish({ body: { outcome: 'session' } }),
  )
  const { screen, router } = await renderLogin()

  await beginAsked.promise
  await signInWithPassword(screen)
  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
  begun.resolve(HttpResponse.json({ options: OPTIONS }))

  await settled()
  expect(router.history.location.pathname).toBe('/login')
  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
})

test('a password login that fails after a Passkey from autofill signed in says nothing', async () => {
  webauthn.autofill.mockResolvedValue(true)
  const ceremony = passkeyCeremony()
  const refused = Promise.withResolvers<Response>()
  worker.use(
    handleSiteAuthLogin(async () => refused.promise),
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish({ body: { outcome: 'session' } }),
  )
  const { screen, router } = await renderLogin()

  await ceremony.started
  await signInWithPassword(screen)
  ceremony.choose()
  await expect.poll(() => router.history.location.pathname).toBe(NEXT)
  refused.resolve(problem(401, { detail: 'Invalid credentials' }))

  await settled()
  expect(screen.getByText('Invalid credentials').query()).toBeNull()
})

test('the code step a provider sent the person back to offers no Passkey in autofill', async () => {
  webauthn.autofill.mockResolvedValue(true)
  // Were the ceremony to start, the person would at once choose a Passkey.
  webauthn.authenticate.mockResolvedValue(ASSERTION)
  worker.use(
    handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }),
    handleSitePasskeyLoginFinish({ body: { outcome: 'session' } }),
  )
  const { screen, router } = await renderLogin('&challenge=signed-challenge')

  await expect.element(screen.getByLabelText(/^Authentication code/)).toBeInTheDocument()
  await settled()
  expect(router.history.location.pathname).toBe('/login')
})

test('the Passkey button waits while a password login is pending', async () => {
  const answer = Promise.withResolvers<Response>()
  worker.use(handleSiteAuthLogin(async () => answer.promise))
  const { screen } = await renderWithRouter(<LoginPage />)

  await signInWithPassword(screen, 'user@example.com')

  await expect.element(screen.getByRole('button', { name: 'Use a passkey' })).toBeDisabled()
  answer.resolve(problem(401, { detail: 'Invalid credentials' }))
  await expect.element(screen.getByRole('button', { name: 'Use a passkey' })).toBeEnabled()
})

test('a password login waits while the Passkey button is pending', async () => {
  const ceremony = passkeyCeremony()
  worker.use(handleSitePasskeyLoginBegin({ body: { options: OPTIONS } }))
  const { screen } = await renderWithRouter(<LoginPage />)

  await screen.getByRole('button', { name: 'Use a passkey' }).click()

  await expect.element(screen.getByRole('button', { name: 'Sign in' })).toBeDisabled()
  await ceremony.started
  ceremony.dismiss()
  await expect.element(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled()
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
