import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAuthSecondFactor,
  handleSiteAuthSigninProviders,
} from '../../generated/site/msw.gen.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { LoginPage } from './login.tsx'
import { RegisterPage } from './register.tsx'

const GOOGLE = { items: [{ name: 'google', displayName: 'Google' }] }

test('the login page offers a button for each enabled sign-in provider', async () => {
  worker.use(handleSiteAuthSigninProviders({ body: GOOGLE }))
  const { screen } = await renderWithRouter(<LoginPage />)

  await expect
    .element(screen.getByRole('link', { name: 'Continue with Google' }))
    .toHaveAttribute('href', '/site/auth/signin/google/start')
})

test('every enabled provider gets its own button, in the order the server lists them', async () => {
  worker.use(
    handleSiteAuthSigninProviders({
      body: {
        items: [
          { name: 'google', displayName: 'Google' },
          { name: 'github', displayName: 'GitHub' },
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(<LoginPage />)

  await expect
    .element(screen.getByRole('link', { name: 'Continue with GitHub' }))
    .toHaveAttribute('href', '/site/auth/signin/github/start')
  await expect.element(screen.getByRole('link', { name: 'Continue with Google' })).toBeVisible()
})

test('the login page hands its return address to the provider flow', async () => {
  worker.use(handleSiteAuthSigninProviders({ body: GOOGLE }))
  const { screen } = await renderWithRouter(<LoginPage />, {
    initialPath: '/workspaces/TestWs01?redirect=%2Fw%2Fabc',
  })

  await expect
    .element(screen.getByRole('link', { name: 'Continue with Google' }))
    .toHaveAttribute('href', '/site/auth/signin/google/start?return_to=%2Fw%2Fabc')
})

test('the register page offers the same buttons', async () => {
  worker.use(handleSiteAuthSigninProviders({ body: GOOGLE }))
  const { screen } = await renderWithRouter(<RegisterPage />)

  await expect.element(screen.getByRole('link', { name: 'Continue with Google' })).toBeVisible()
})

test('with no provider enabled the login page has no provider button', async () => {
  const { screen } = await renderWithRouter(<LoginPage />)

  await expect.element(screen.getByRole('button', { name: 'Sign in' })).toBeVisible()
  await expect.element(screen.getByText('Continue with')).not.toBeInTheDocument()
})

test('a provider sign-in that needs a second factor opens the code step', async () => {
  const secondStepBodies: unknown[] = []
  worker.use(
    handleSiteAuthSecondFactor(async ({ request }) => {
      secondStepBodies.push(await request.json())
      return HttpResponse.json({ outcome: 'session' })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<LoginPage />, {
    initialPath: '/workspaces/TestWs01?challenge=signed-challenge',
  })

  await screen.getByLabelText(/^Authentication code/).fill('123456')
  await screen.getByRole('button', { name: 'Verify' }).click()

  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: '/' }])
  expect(secondStepBodies).toEqual([{ challenge: 'signed-challenge', code: '123456' }])
})

test('a refused provider sign-in is explained on the login page', async () => {
  const { screen } = await renderWithRouter(<LoginPage />, {
    initialPath: '/workspaces/TestWs01?error=email_not_trusted',
  })

  await expect
    .element(screen.getByText(/does not vouch for that email address/))
    .toBeInTheDocument()
})
