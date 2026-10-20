import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import { handleSiteAuthVerifyEmail } from '../../generated/site/msw.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as verifyEmailRoute } from '../../routes/verify-email.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { VerifyEmailPage } from './verify-email.tsx'

const LANDING = 'JoinedWs'
const mount = routeMount(verifyEmailRoute)
const MOUNT = { ...mount, initialPath: `${mount.initialPath}?token=tok-v` }

async function fillAndSubmit(
  screen: Awaited<ReturnType<typeof renderWithRouter>>['screen'],
  confirm: string,
) {
  await screen.getByLabelText(/^Password/).fill('s3cret-pass')
  await screen.getByLabelText(/^Confirm password/).fill(confirm)
  await screen.getByRole('button', { name: 'Set password' }).click()
}

test('sends the token alone first and opens the workspace the answer names when it is accepted', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthVerifyEmail(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({ landingWorkspaceId: LANDING })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await vi.waitFor(() =>
    expect(navigate).toHaveBeenCalledWith({
      to: overviewRoute.to,
      params: { workspaceId: LANDING },
      replace: true,
    }),
  )
  expect(bodies).toEqual([{ token: 'tok-v' }])
  await expect.element(screen.getByRole('button', { name: 'Set password' })).not.toBeInTheDocument()
})

test('asks for a password when the link is not in the registration browser and resends with it', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthVerifyEmail(async ({ request }) => {
      const body = await request.json()
      bodies.push(body)
      if ((body as { password?: string }).password === undefined) {
        return problem(428, { detail: 'set a password' })
      }
      return HttpResponse.json({ landingWorkspaceId: LANDING })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await fillAndSubmit(screen, 's3cret-pass')

  await vi.waitFor(() =>
    expect(navigate).toHaveBeenCalledWith({
      to: overviewRoute.to,
      params: { workspaceId: LANDING },
      replace: true,
    }),
  )
  expect(bodies).toEqual([{ token: 'tok-v' }, { token: 'tok-v', password: 's3cret-pass' }])
})

test('does not send a password when the confirmation differs', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthVerifyEmail(async ({ request }) => {
      bodies.push(await request.json())
      return problem(428, { detail: 'set a password' })
    }),
  )
  const { screen } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await fillAndSubmit(screen, 'something-else')

  await expect.element(screen.getByText('Passwords do not match')).toBeInTheDocument()
  expect(bodies).toEqual([{ token: 'tok-v' }])
})

test('shows the invalid-link message with a sign-in link when the link is rejected', async () => {
  worker.use(
    handleSiteAuthVerifyEmail(() =>
      problem(400, { detail: 'invalid or expired verification link' }),
    ),
  )
  const { screen } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await expect
    .element(screen.getByText('This verification link is invalid or has expired.'))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Set password' })).not.toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Sign in' }))
    .toHaveAttribute('href', loginRoute.to)
})

test('offers no form when the address carries no token', async () => {
  const { screen } = await renderWithRouter(<VerifyEmailPage />, {
    ...mount,
    initialPath: `${mount.initialPath}?token=`,
  })

  await expect
    .element(screen.getByText('This verification link is invalid or has expired.'))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Set password' })).not.toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Sign in' }))
    .toHaveAttribute('href', loginRoute.to)
})

test('offers a retry that sends the token again after a server failure', async () => {
  let calls = 0
  worker.use(
    handleSiteAuthVerifyEmail(() => {
      calls += 1
      return calls === 1
        ? problem(500, { detail: 'boom' })
        : HttpResponse.json({ landingWorkspaceId: LANDING })
    }),
  )
  const { screen, navigate } = await renderWithRouter(<VerifyEmailPage />, MOUNT)

  await screen.getByRole('button', { name: 'Try again' }).click()

  await vi.waitFor(() =>
    expect(navigate).toHaveBeenCalledWith({
      to: overviewRoute.to,
      params: { workspaceId: LANDING },
      replace: true,
    }),
  )
  expect(calls).toBe(2)
})
