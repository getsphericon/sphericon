import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAuthResetPassword } from '../../generated/site/msw.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as resetPasswordRoute } from '../../routes/reset-password.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ResetPasswordPage } from './reset-password.tsx'

const mount = routeMount(resetPasswordRoute)
const MOUNT = { ...mount, initialPath: `${mount.initialPath}?token=tok-9` }

async function fill(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  password: string,
  confirm: string,
) {
  await screen.getByLabelText(/^New password/).fill(password)
  await screen.getByLabelText(/^Confirm new password/).fill(confirm)
  await screen.getByRole('button', { name: 'Set password' }).click()
}

test('sends the token and new password, then navigates to login', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthResetPassword(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen, navigate } = await renderWithRouter(<ResetPasswordPage />, MOUNT)

  await fill(screen, 'new-secret', 'new-secret')

  await expect
    .element(screen.getByText('Your password has been reset. Please sign in.'))
    .toBeInTheDocument()
  expect(bodies).toEqual([{ token: 'tok-9', password: 'new-secret' }])
  await expect.poll(() => navigate.mock.calls).toContainEqual([{ to: loginRoute.to }])
})

test('rejects mismatching passwords without calling the API', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthResetPassword(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen, navigate } = await renderWithRouter(<ResetPasswordPage />, MOUNT)

  await fill(screen, 'one', 'two')

  await expect.element(screen.getByText('Passwords do not match')).toBeInTheDocument()
  expect(bodies).toEqual([])
  expect(navigate).not.toHaveBeenCalled()
})

test('shows the API error and clears both fields on failure', async () => {
  worker.use(handleSiteAuthResetPassword(() => problem(400, { detail: 'Token expired' })))
  const { screen, navigate } = await renderWithRouter(<ResetPasswordPage />, MOUNT)

  await fill(screen, 'new-secret', 'new-secret')

  await expect.element(screen.getByText('Token expired')).toBeInTheDocument()
  await expect.element(screen.getByLabelText(/^New password/)).toHaveValue('')
  await expect.element(screen.getByLabelText(/^Confirm new password/)).toHaveValue('')
  expect(navigate).not.toHaveBeenCalled()
})

test('falls back to the generic message when the error has no detail', async () => {
  worker.use(handleSiteAuthResetPassword(() => problem(500, {})))
  const { screen } = await renderWithRouter(<ResetPasswordPage />, MOUNT)

  await fill(screen, 'a', 'a')

  await expect
    .element(
      screen.getByText(
        "We couldn't reset your password. The link may have expired — request a new one.",
      ),
    )
    .toBeInTheDocument()
})
