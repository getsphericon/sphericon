import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import { handleSiteAuthConfirmEmailChange } from '../../generated/site/msw.gen.ts'
import { Route as confirmEmailChangeRoute } from '../../routes/confirm-email-change.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { ConfirmEmailChangePage } from './confirm-email-change.tsx'

const mount = routeMount(confirmEmailChangeRoute)
const MOUNT = { ...mount, initialPath: `${mount.initialPath}?token=tok-c` }

test('confirms the new email on mount and links to sign in', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthConfirmEmailChange(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<ConfirmEmailChangePage />, MOUNT)

  await expect.element(screen.getByText('Email updated')).toBeInTheDocument()
  expect(bodies).toEqual([{ token: 'tok-c' }])
  await expect
    .element(screen.getByRole('link', { name: 'Go to sign in' }))
    .toHaveAttribute('href', loginRoute.to)
})

test('shows a loading state while the request is in flight', async () => {
  worker.use(handleSiteAuthConfirmEmailChange(async () => new Promise<never>(vi.fn<() => void>())))
  const { screen } = await renderWithRouter(<ConfirmEmailChangePage />, MOUNT)

  await expect.element(screen.getByText('Confirming your new email…')).toBeInTheDocument()
})

test('shows the failure state when the token is rejected', async () => {
  worker.use(handleSiteAuthConfirmEmailChange(() => problem(400, {})))
  const { screen } = await renderWithRouter(<ConfirmEmailChangePage />, MOUNT)

  await expect.element(screen.getByText('Confirmation failed')).toBeInTheDocument()
  await expect
    .element(screen.getByText('This confirmation link is invalid or has expired.'))
    .toBeInTheDocument()
})
