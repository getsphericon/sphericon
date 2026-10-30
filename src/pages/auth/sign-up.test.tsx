import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import {
  handleSiteAuthSignUp,
  handleSiteAuthResendVerification,
  handleSiteLegalDocumentsSignupItems,
} from '../../generated/site/msw.gen.ts'
import { SignupItemKind } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { SignUpPage } from './sign-up.tsx'

async function fillAndSubmit(screen: Awaited<ReturnType<typeof renderWithRouter>>['screen']) {
  await screen.getByLabelText(/^Name/).fill('  Ada Lovelace ')
  await screen.getByLabelText(/^Email/).fill(' ada@example.com ')
  await screen.getByLabelText(/^Password/).fill('correct horse battery')
  await screen.getByLabelText(/^Confirm password/).fill('correct horse battery')
  await screen.getByRole('button', { name: 'Sign up' }).click()
}

test('signs up with trimmed name and email and the password, then asks to check the email', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSignUp(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<SignUpPage />)

  await fillAndSubmit(screen)

  await expect.element(screen.getByText('Check your email')).toBeInTheDocument()
  await expect.element(screen.getByText(/ada@example\.com/)).toBeInTheDocument()
  expect(bodies).toEqual([
    { name: 'Ada Lovelace', email: 'ada@example.com', password: 'correct horse battery' },
  ])
})

test('shows the API error and stays on the form when the Sign-up fails', async () => {
  worker.use(handleSiteAuthSignUp(() => problem(422, { detail: 'Name is required' })))
  const { screen } = await renderWithRouter(<SignUpPage />)

  await fillAndSubmit(screen)

  await expect.element(screen.getByText('Name is required')).toBeInTheDocument()
  await expect.element(screen.getByText('Sign-up failed')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Sign up' })).toBeInTheDocument()
})

test('falls back to the generic message when the error has no detail', async () => {
  worker.use(handleSiteAuthSignUp(() => problem(500, {})))
  const { screen } = await renderWithRouter(<SignUpPage />)

  await fillAndSubmit(screen)

  await expect
    .element(
      screen.getByText("We couldn't create your account. Please check your details and try again."),
    )
    .toBeInTheDocument()
})

test('links back to sign in', async () => {
  const { screen } = await renderWithRouter(<SignUpPage />)
  await expect
    .element(screen.getByRole('link', { name: 'Already have an account? Sign in' }))
    .toBeInTheDocument()
})

afterEach(() => {
  Reflect.deleteProperty(window, '__REQUIRE_EMAIL_VERIFICATION__')
})

test('with the gate off it asks for a password, sends it and points to sign in', async () => {
  Reflect.set(window, '__REQUIRE_EMAIL_VERIFICATION__', false)
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSignUp(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<SignUpPage />)

  await screen.getByLabelText(/^Name/).fill('Ada Lovelace')
  await screen.getByLabelText(/^Email/).fill('ada@example.com')
  await screen.getByLabelText(/^Password/).fill('correct horse battery')
  await screen.getByLabelText(/^Confirm password/).fill('correct horse battery')
  await screen.getByRole('button', { name: 'Sign up' }).click()

  await expect.element(screen.getByText('Account created')).toBeInTheDocument()
  expect(bodies).toEqual([
    { name: 'Ada Lovelace', email: 'ada@example.com', password: 'correct horse battery' },
  ])
})

test('sends nothing when the confirmation differs', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSignUp(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<SignUpPage />)

  await screen.getByLabelText(/^Name/).fill('Ada Lovelace')
  await screen.getByLabelText(/^Email/).fill('ada@example.com')
  await screen.getByLabelText(/^Password/).fill('correct horse battery')
  await screen.getByLabelText(/^Confirm password/).fill('something else')
  await screen.getByRole('button', { name: 'Sign up' }).click()

  await expect.element(screen.getByText('Passwords do not match')).toBeInTheDocument()
  expect(bodies).toEqual([])
})

test('resends the verification link to the address that signed up from the check-email screen', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSignUp(() => new HttpResponse(null, { status: 202 })),
    handleSiteAuthResendVerification(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<SignUpPage />)

  await fillAndSubmit(screen)
  await screen.getByRole('button', { name: 'Resend verification email' }).click()

  await expect
    .element(
      screen.getByText('If this address is waiting for verification, we sent the link again'),
    )
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: /^Resend in \d+s$/ })).toBeDisabled()
  expect(bodies).toEqual([{ email: 'ada@example.com' }])
})

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

test('asks each Sign-up item unticked, links each to its page, and sends the answers', async () => {
  worker.use(handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }))
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSignUp(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<SignUpPage />)

  const terms = screen.getByRole('checkbox', { name: 'Terms of service' })
  await expect.element(terms).not.toBeChecked()
  await expect.element(screen.getByRole('checkbox', { name: 'News from us' })).not.toBeChecked()
  await expect
    .element(screen.getByRole('link', { name: 'How we use your data' }))
    .toHaveAttribute('href', '/legal/data-notice')
  await expect
    .element(screen.getByRole('link', { name: 'Read the Terms of service' }))
    .toHaveAttribute('href', '/legal/terms')
  await terms.click()
  await fillAndSubmit(screen)

  await expect.element(screen.getByText('Check your email')).toBeInTheDocument()
  expect(bodies).toEqual([
    {
      name: 'Ada Lovelace',
      email: 'ada@example.com',
      password: 'correct horse battery',
      signupItems: [
        { key: 'terms', version: '2026-02', accepted: true },
        { key: 'news', version: '2026-01', accepted: false },
        { key: 'data-notice', version: '2026-01' },
      ],
    },
  ])
})

test('shows the refusal of a required acceptance under its box', async () => {
  worker.use(handleSiteLegalDocumentsSignupItems({ body: { items: SIGNUP_ITEMS } }))
  worker.use(
    handleSiteAuthSignUp(() =>
      problem(422, {
        detail: 'accept the Terms of service to sign up',
        errors: { 'signupItems.terms': ['accept the Terms of service to sign up'] },
      }),
    ),
  )
  const { screen } = await renderWithRouter(<SignUpPage />)

  await expect.element(screen.getByRole('checkbox', { name: 'Terms of service' })).toBeVisible()
  await fillAndSubmit(screen)

  await expect
    .element(screen.getByRole('checkbox', { name: 'Terms of service' }))
    .toHaveAccessibleDescription(/accept the Terms of service to sign up/)
})
