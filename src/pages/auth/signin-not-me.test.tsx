import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteAuthSigninNotMe } from '../../generated/site/msw.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as signinNotMeRoute } from '../../routes/signin-not-me.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SigninNotMePage } from './signin-not-me.tsx'

const mount = routeMount(signinNotMeRoute)
const MOUNT = { ...mount, initialPath: `${mount.initialPath}?token=tok-n` }

test('unlinks the provider on mount and links to sign in', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAuthSigninNotMe(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json({})
    }),
  )
  const { screen } = await renderWithRouter(<SigninNotMePage />, MOUNT)

  await expect.element(screen.getByText('Provider unlinked')).toBeInTheDocument()
  expect(bodies).toEqual([{ token: 'tok-n' }])
  await expect
    .element(screen.getByRole('link', { name: 'Go to sign in' }))
    .toHaveAttribute('href', loginRoute.to)
})

test('shows the failure state when the link is rejected', async () => {
  worker.use(handleSiteAuthSigninNotMe(() => problem(400, {})))
  const { screen } = await renderWithRouter(<SigninNotMePage />, MOUNT)

  await expect.element(screen.getByText('Link not accepted')).toBeInTheDocument()
})
