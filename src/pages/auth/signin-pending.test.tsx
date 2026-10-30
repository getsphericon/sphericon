import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAuthSigninPending,
  handleSiteAuthSigninPendingDecline,
  handleSiteAuthSigninPendingSubmit,
  handleSiteLegalDocumentsSignupItems,
} from '../../generated/site/msw.gen.ts'
import { SignupItemKind } from '../../generated/site/types.gen.ts'
import { Route as signUpRoute } from '../../routes/sign-up.tsx'
import { Route as signinPendingRoute } from '../../routes/signin-pending.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SigninPendingPage } from './signin-pending.tsx'

const mount = routeMount(signinPendingRoute)
const MOUNT = { ...mount, initialPath: `${mount.initialPath}?token=tok-p&redirect=%2Fw%2Fnext` }

// The Sign-up items of an Instance that has them (ADR 0062): one of each kind.
const SIGNUP_ITEMS = [
  {
    key: 'terms',
    title: 'Terms of service',
    kind: SignupItemKind.REQUIRED_ACCEPTANCE,
    version: '2026-02',
  },
  { key: 'news', title: 'News from us', kind: SignupItemKind.OPTIONAL_CONSENT, version: '2026-01' },
  {
    key: 'data-notice',
    title: 'How we use your data',
    kind: SignupItemKind.NOTICE,
    version: '2026-01',
  },
]

function pendingSignup() {
  worker.use(
    handleSiteAuthSigninPending({
      body: { provider: 'Google', email: 'nina@gmail.com', name: 'Nina' },
    }),
    handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }),
  )
}

test('shows who signs up, asks the items unticked and completes the sign-up with the answers', async () => {
  pendingSignup()
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSigninPendingSubmit(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<SigninPendingPage />, MOUNT)

  await expect.element(screen.getByText(/nina@gmail\.com with Google/)).toBeInTheDocument()
  const terms = screen.getByRole('checkbox', { name: 'Terms of service' })
  await expect.element(terms).not.toBeChecked()
  await expect.element(screen.getByRole('checkbox', { name: 'News from us' })).not.toBeChecked()
  await expect
    .element(screen.getByRole('link', { name: 'How we use your data' }))
    .toHaveAttribute('href', '/legal/data-notice')
  await terms.click()
  await screen.getByRole('button', { name: 'Create account' }).click()

  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate).toHaveBeenCalledWith({ href: '/w/next' })
  expect(bodies).toEqual([
    {
      token: 'tok-p',
      signupItems: [
        { key: 'terms', version: '2026-02', accepted: true },
        { key: 'news', version: '2026-01', accepted: false },
        { key: 'data-notice', version: '2026-01' },
      ],
    },
  ])
})

test('shows the refusal of a required acceptance under its box', async () => {
  pendingSignup()
  worker.use(
    handleSiteAuthSigninPendingSubmit(() =>
      problem(422, {
        detail: 'accept the Terms of service to sign up',
        errors: { 'signupItems.terms': ['accept the Terms of service to sign up'] },
      }),
    ),
  )
  const { screen, navigate } = await renderWithRouter(<SigninPendingPage />, MOUNT)

  await screen.getByRole('button', { name: 'Create account' }).click()

  await expect
    .element(screen.getByRole('checkbox', { name: 'Terms of service' }))
    .toHaveAccessibleDescription(/accept the Terms of service to sign up/)
  expect(navigate).not.toHaveBeenCalled()
})

test('declining sends the reference and goes back to sign up', async () => {
  pendingSignup()
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSigninPendingDecline(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<SigninPendingPage />, MOUNT)

  await screen.getByRole('button', { name: 'Cancel' }).click()

  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate).toHaveBeenCalledWith({ to: signUpRoute.to })
  expect(bodies).toEqual([{ token: 'tok-p' }])
})

test('an expired sign-up says so and offers to start again', async () => {
  worker.use(
    handleSiteAuthSigninPending(() => problem(400, { detail: 'this sign-up has expired' })),
    handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }),
  )
  const { screen } = await renderWithRouter(<SigninPendingPage />, MOUNT)

  await expect.element(screen.getByText('This sign-up has expired')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Sign up again' }))
    .toHaveAttribute('href', signUpRoute.to)
  await expect
    .element(screen.getByRole('button', { name: 'Create account' }))
    .not.toBeInTheDocument()
})
