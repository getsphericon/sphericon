import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteAuthSigninProviders,
  handleSitePasskeysList,
  handleSiteSecondFactorConfirmEnrollment,
  handleSiteSecondFactorDisable,
  handleSiteSecondFactorGetStatus,
  handleSiteSecondFactorRegenerateRecoveryCodes,
  handleSiteSecondFactorStartEnrollment,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteSecondFactorEnrollment,
  SiteSecondFactorStatus,
} from '../../generated/site/types.gen.ts'
import { Route as securityRoute } from '../../routes/account/security.tsx'
import { testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SecurityPage } from './security.tsx'

type Op = 'start' | 'confirm' | 'regenerate' | 'disable'
interface Call {
  op: Op
  body: unknown
}
type Handler = (req: Request) => Response | Promise<Response>

const enrollment: SiteSecondFactorEnrollment = {
  enrollmentId: testId(1),
  secret: 'JBSWY3DPEHPK3PXP',
  otpauthUri:
    'otpauth://totp/sphericon:info@getsphericon.com?secret=JBSWY3DPEHPK3PXP&issuer=sphericon',
  qrCode:
    'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==',
}
const codes = ['aaaaa-bbbbb', 'ccccc-ddddd'] as const

// Serves the Second factor endpoints with a status the mutations move, and records
// every mutating call. A held operation answers once its promise settles. Each Start issues the
// next enrollment id, as the server does: the first is `enrollment`'s.
function serveSecurity(
  calls: Call[],
  initial: SiteSecondFactorStatus,
  {
    overrides = {},
    hold = {},
  }: {
    overrides?: Partial<Record<Op, Handler>>
    hold?: Partial<Record<Op, Promise<unknown>>>
  } = {},
) {
  let status = initial
  let started = 0
  const serve =
    (op: Op, fallback: Handler): Handler =>
    async (req) => {
      const text = await req.clone().text()
      calls.push({ op, body: text ? JSON.parse(text) : null })
      await hold[op]
      return (overrides[op] ?? fallback)(req)
    }
  worker.use(
    handleSiteSecondFactorGetStatus(() => HttpResponse.json(status)),
    handleSiteSecondFactorStartEnrollment(async ({ request }) =>
      serve('start', () => {
        status = { ...initial, enabled: false, pending: true, recoveryCodesRemaining: 0 }
        started += 1
        return HttpResponse.json({ ...enrollment, enrollmentId: testId(started) })
      })(request),
    ),
    handleSiteSecondFactorConfirmEnrollment(async ({ request }) =>
      serve('confirm', () => {
        status = { ...initial, enabled: true, pending: false, recoveryCodesRemaining: codes.length }
        return HttpResponse.json({ codes })
      })(request),
    ),
    handleSiteSecondFactorRegenerateRecoveryCodes(async ({ request }) =>
      serve('regenerate', () => HttpResponse.json({ codes }))(request),
    ),
    handleSiteSecondFactorDisable(async ({ request }) =>
      serve('disable', () => {
        status = { ...initial, enabled: false, pending: false, recoveryCodesRemaining: 0 }
        return new HttpResponse(null, { status: 204 })
      })(request),
    ),
  )
}

const withPassword = { passwordSet: true, identityProofFresh: false }
const off: SiteSecondFactorStatus = {
  enabled: false,
  pending: false,
  recoveryCodesRemaining: 0,
  ...withPassword,
}
const on: SiteSecondFactorStatus = {
  enabled: true,
  pending: false,
  recoveryCodesRemaining: 3,
  ...withPassword,
}

const renderPage = async () => renderWithRouter(<SecurityPage />, routeMount(securityRoute))

type Screen = Awaited<ReturnType<typeof renderPage>>['screen']

// Starts the setup with the password and types the code from the app, short of Turn on.
async function enterEnrollment(screen: Screen) {
  await screen.getByRole('button', { name: 'Set up authenticator app' }).click()
  await screen.getByLabelText(/^Current password/).fill('pw-123456')
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Code from the app/).fill('123456')
}

test('enrolls an authenticator app and shows the recovery codes once', async () => {
  const calls: Call[] = []
  serveSecurity(calls, off)
  const { screen } = await renderPage()

  await expect.element(screen.getByText('Off', { exact: true })).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Set up authenticator app' }).click()
  await screen.getByLabelText(/^Current password/).fill('pw-123456')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect
    .element(screen.getByRole('img', { name: 'QR code for the authenticator app' }))
    .toBeInTheDocument()
  await expect.element(screen.getByText(enrollment.secret)).toBeInTheDocument()

  await screen.getByLabelText(/^Code from the app/).fill('123456')
  await screen.getByRole('button', { name: 'Turn on' }).click()

  await expect.element(screen.getByText('Save your recovery codes')).toBeInTheDocument()
  await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
  await Promise.all(
    codes.map(async (code) => {
      await expect.element(screen.getByText(code)).toBeInTheDocument()
    }),
  )
  expect(calls).toEqual([
    { op: 'start', body: { currentPassword: 'pw-123456' } },
    {
      op: 'confirm',
      body: { currentPassword: 'pw-123456', enrollmentId: enrollment.enrollmentId, code: '123456' },
    },
  ])

  await screen.getByRole('button', { name: 'I have saved them' }).click()
  expect(screen.getByText(codes[0]).elements()).toHaveLength(0)
  await expect.element(screen.getByText('On', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('2 recovery codes left')).toBeInTheDocument()
})

test('shows the recovery codes once the confirmed status has loaded', async () => {
  serveSecurity([], off)
  const { screen } = await renderPage()
  await enterEnrollment(screen)

  // The status refetch the confirmation sets off is held until the test lets it answer.
  const refetch = Promise.withResolvers<undefined>()
  let refetched = false
  worker.use(
    handleSiteSecondFactorGetStatus(async () => {
      refetched = true
      await refetch.promise
      return HttpResponse.json({ ...on, recoveryCodesRemaining: codes.length })
    }),
  )
  await screen.getByRole('button', { name: 'Turn on' }).click()

  // Confirm has answered and the status is being refetched: no codes yet.
  await expect.poll(() => refetched).toBe(true)
  const codesBeforeStatus = screen.getByText('Save your recovery codes').elements()
  expect(codesBeforeStatus).toHaveLength(0)

  // The refetch answers `enabled: true`, and only then are the codes shown.
  refetch.resolve(undefined)
  await expect.element(screen.getByText('Save your recovery codes')).toBeInTheDocument()
  await expect.element(screen.getByText(codes[0])).toBeInTheDocument()
  await expect.element(screen.getByText('On', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
})

test('shows the recovery codes when the setup is closed while the confirmation is pending', async () => {
  const confirmed = Promise.withResolvers<undefined>()
  serveSecurity([], off, { hold: { confirm: confirmed.promise } })
  const { screen } = await renderPage()
  await enterEnrollment(screen)
  await screen.getByRole('button', { name: 'Turn on' }).click()

  await expect.element(screen.getByRole('button', { name: 'Turn on' })).toBeDisabled()
  await userEvent.keyboard('{Escape}')
  await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()

  confirmed.resolve(undefined)
  await expect.element(screen.getByText('Save your recovery codes')).toBeInTheDocument()
  await expect.element(screen.getByText(codes[0])).toBeInTheDocument()
})

test('a setup started again while the earlier confirmation is pending can be confirmed', async () => {
  const stale = Promise.withResolvers<undefined>()
  serveSecurity([], off, {
    // The server answers the earlier enrollment once a later Start has replaced it.
    overrides: { confirm: () => problem(409, { detail: 'the enrollment was replaced' }) },
    hold: { confirm: stale.promise },
  })
  const { screen } = await renderPage()
  await enterEnrollment(screen)
  await screen.getByRole('button', { name: 'Turn on' }).click()
  await expect.element(screen.getByRole('button', { name: 'Turn on' })).toBeDisabled()
  await userEvent.keyboard('{Escape}')

  await enterEnrollment(screen)
  await expect.element(screen.getByRole('button', { name: 'Turn on' })).toBeEnabled()

  stale.resolve(undefined)
})

test('reports a wrong confirmation code and keeps the setup open', async () => {
  serveSecurity([], off, {
    overrides: { confirm: () => problem(422, { detail: 'the code is not valid' }) },
  })
  const { screen } = await renderPage()

  await enterEnrollment(screen)
  await screen.getByRole('button', { name: 'Turn on' }).click()

  await expect
    .element(screen.getByText('Could not turn on two-factor authentication'))
    .toBeInTheDocument()
  await expect.element(screen.getByText(enrollment.secret)).toBeInTheDocument()
  expect(screen.getByText('Save your recovery codes').elements()).toHaveLength(0)
})

test('regenerates the recovery codes with the password', async () => {
  const calls: Call[] = []
  serveSecurity(calls, on)
  const { screen } = await renderPage()

  await expect.element(screen.getByText('3 recovery codes left')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Generate new codes' }).click()
  await screen.getByLabelText(/^Current password/).fill('pw-123456')
  await screen.getByRole('dialog').getByRole('button', { name: 'Generate new codes' }).click()

  await expect.element(screen.getByText(codes[1])).toBeInTheDocument()
  await expect.element(screen.getByRole('dialog')).not.toBeInTheDocument()
  expect(calls).toEqual([{ op: 'regenerate', body: { currentPassword: 'pw-123456' } }])
})

test('disables the second factor with the password and a code', async () => {
  const calls: Call[] = []
  serveSecurity(calls, on)
  const { screen } = await renderPage()

  await screen.getByRole('button', { name: 'Turn off two-factor authentication' }).click()
  await screen.getByLabelText(/^Current password/).fill('pw-123456')
  await screen.getByLabelText(/^Code from the app/).fill('654321')
  await screen
    .getByRole('dialog')
    .getByRole('button', { name: 'Turn off two-factor authentication' })
    .click()

  await expect.element(screen.getByText('Two-factor authentication is off')).toBeInTheDocument()
  expect(calls).toEqual([{ op: 'disable', body: { currentPassword: 'pw-123456', code: '654321' } }])
  await expect
    .element(screen.getByRole('button', { name: 'Set up authenticator app' }))
    .toBeInTheDocument()
})

const passwordless = { passwordSet: false, identityProofFresh: true }

test('a User with no password enrolls with a fresh provider login and no password field', async () => {
  const calls: Call[] = []
  serveSecurity(calls, { ...off, ...passwordless })
  const { screen } = await renderPage()

  await expect
    .element(screen.getByRole('button', { name: 'Set up authenticator app' }))
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Set up authenticator app' }).click()
  await expect.element(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument()
  expect(screen.getByLabelText(/^Current password/).elements()).toHaveLength(0)
  await screen.getByRole('button', { name: 'Continue' }).click()
  await screen.getByLabelText(/^Code from the app/).fill('123456')
  await screen.getByRole('button', { name: 'Turn on' }).click()

  await expect.element(screen.getByText('Save your recovery codes')).toBeInTheDocument()
  expect(calls).toEqual([
    { op: 'start', body: {} },
    { op: 'confirm', body: { enrollmentId: enrollment.enrollmentId, code: '123456' } },
  ])
})

test('a User with no password and a Passkey is offered a Passkey check for the Second factor', async () => {
  serveSecurity([], { ...off, passwordSet: false, identityProofFresh: false })
  worker.use(
    handleSitePasskeysList({
      body: {
        items: [{ id: testId(1), name: 'My laptop', createdAt: '2026-01-01T00:00:00Z' }],
        available: true,
        passwordSet: false,
        identityProofFresh: false,
      },
    }),
  )
  const { screen } = await renderPage()

  await expect
    .element(screen.getByRole('button', { name: 'Verify with a passkey' }).first())
    .toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Set up authenticator app' }).elements()).toHaveLength(
    0,
  )
})

test('a User with no password and an old provider login is sent to sign in with a provider again', async () => {
  serveSecurity([], { ...off, passwordSet: false, identityProofFresh: false })
  worker.use(
    handleSiteAuthSigninProviders({ body: { items: [{ name: 'google', displayName: 'Google' }] } }),
  )
  const { screen } = await renderPage()

  await expect.element(screen.getByText('Confirm it is you')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Continue with Google' }))
    .toHaveAttribute('href', '/site/auth/signin/google/start?return_to=%2Faccount%2Fsecurity')
  expect(screen.getByRole('button', { name: 'Set up authenticator app' }).elements()).toHaveLength(
    0,
  )
})
