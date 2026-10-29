import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import {
  handleSiteAuthSignUp,
  handleSiteAuthResendVerification,
} from '../../generated/site/msw.gen.ts'
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
