import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'

import { handleSiteAuthRegister } from '../../generated/site/msw.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { RegisterPage } from './register.tsx'

async function fillAndSubmit(screen: Awaited<ReturnType<typeof renderWithRouter>>['screen']) {
  await screen.getByLabelText(/^Name/).fill('  Ada Lovelace ')
  await screen.getByLabelText(/^Email/).fill(' ada@example.com ')
  await screen.getByRole('button', { name: 'Register' }).click()
}

test('registers with trimmed name and email only, then asks to check the email', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthRegister(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<RegisterPage />)

  await fillAndSubmit(screen)

  await expect.element(screen.getByText('Check your email')).toBeInTheDocument()
  await expect.element(screen.getByText(/ada@example\.com/)).toBeInTheDocument()
  expect(bodies).toEqual([{ name: 'Ada Lovelace', email: 'ada@example.com' }])
})

test('shows the API error and stays on the form when registration fails', async () => {
  worker.use(handleSiteAuthRegister(() => problem(422, { detail: 'Name is required' })))
  const { screen } = await renderWithRouter(<RegisterPage />)

  await fillAndSubmit(screen)

  await expect.element(screen.getByText('Name is required')).toBeInTheDocument()
  await expect.element(screen.getByText('Registration failed')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Register' })).toBeInTheDocument()
})

test('falls back to the generic message when the error has no detail', async () => {
  worker.use(handleSiteAuthRegister(() => problem(500, {})))
  const { screen } = await renderWithRouter(<RegisterPage />)

  await fillAndSubmit(screen)

  await expect
    .element(
      screen.getByText("We couldn't create your account. Please check your details and try again."),
    )
    .toBeInTheDocument()
})

test('links back to sign in', async () => {
  const { screen } = await renderWithRouter(<RegisterPage />)
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
    handleSiteAuthRegister(async ({ request }) => {
      bodies.push(await request.json())
      return new HttpResponse(null, { status: 202 })
    }),
  )
  const { screen } = await renderWithRouter(<RegisterPage />)

  await screen.getByLabelText(/^Name/).fill('Ada Lovelace')
  await screen.getByLabelText(/^Email/).fill('ada@example.com')
  await screen.getByLabelText(/^Password/).fill('correct horse battery')
  await screen.getByRole('button', { name: 'Register' }).click()

  await expect.element(screen.getByText('Account created')).toBeInTheDocument()
  expect(bodies).toEqual([
    { name: 'Ada Lovelace', email: 'ada@example.com', password: 'correct horse battery' },
  ])
})

test('with the gate on there is no password field', async () => {
  const { screen } = await renderWithRouter(<RegisterPage />)

  await expect.element(screen.getByLabelText(/^Name/)).toBeInTheDocument()
  expect(screen.getByLabelText(/^Password/).elements()).toHaveLength(0)
})
