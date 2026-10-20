import { QueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'

import { handleSiteWorkspacesList } from './generated/site/msw.gen.ts'
import { handleStaffMeGet } from './generated/staff/msw.gen.ts'
import { createAppRouter } from './router.ts'
import { Route as consoleHomeRoute } from './routes/console/_authed/index.tsx'
import { Route as consoleLoginRoute } from './routes/console/login.tsx'
import { Route as indexRoute } from './routes/index.tsx'
import { Route as loginRoute } from './routes/login.tsx'
import { Route as oauthConsentRoute } from './routes/oauth/consent.tsx'
import { Route as workspaceRoute } from './routes/workspaces/$workspaceId/route.tsx'
import { Route as newWorkspaceRoute } from './routes/workspaces/new.tsx'
import { problem } from './test/problem.ts'
import { worker } from './test/worker.ts'

// A router with a cache of its own: the guards read the workspaces through it. A guard's redirect
// starts a second View Transition that skips the first, whose rejected promise nobody awaits and
// vitest reports as an unhandled error, so the cross-fade is switched off here.
function newRouter() {
  const router = createAppRouter(new QueryClient({ defaultOptions: { queries: { retry: false } } }))
  router.update({ defaultViewTransition: false })
  return router
}

const workspace = (workspaceId: string) => ({
  id: workspaceId,
  name: workspaceId,
  workspaceId,
  collectKey: 'omck',
  postalAddress: '',
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
})

// The guards only ever call the workspaces list, so one stub per scenario is enough.
function serveWorkspaces(ids: string[] | 'unauthorized') {
  worker.use(
    ids === 'unauthorized'
      ? handleSiteWorkspacesList(() => problem(401, { detail: 'unauthorized' }))
      : handleSiteWorkspacesList({ body: ids.map((workspaceId) => workspace(workspaceId)) }),
  )
}

test('the index route sends a signed-in user to their first workspace', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01', 'OtherWs3'])

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(
    workspaceRoute.fullPath.replace('$workspaceId', 'AcmeWs01'),
  )
})

test('the index route sends a user without workspaces to the create screen', async () => {
  const router = newRouter()
  serveWorkspaces([])

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(newWorkspaceRoute.to)
})

test('the index route sends an unauthorized user to login', async () => {
  const router = newRouter()
  serveWorkspaces('unauthorized')

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(loginRoute.to)
})

test('an unknown workspace workspaceId falls back to the first workspace', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])

  await router.navigate({ to: workspaceRoute.to, params: { workspaceId: 'nope' } })

  expect(router.state.location.pathname).toBe(
    workspaceRoute.fullPath.replace('$workspaceId', 'AcmeWs01'),
  )
})

test('an unknown workspace workspaceId with no workspaces goes to the create screen', async () => {
  const router = newRouter()
  serveWorkspaces([])

  await router.navigate({ to: workspaceRoute.to, params: { workspaceId: 'nope' } })

  expect(router.state.location.pathname).toBe(newWorkspaceRoute.to)
})

test('the console sends a visitor without a staff member session to the console login', async () => {
  const router = newRouter()
  worker.use(handleStaffMeGet(() => problem(401, { detail: 'unauthorized' })))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe(consoleLoginRoute.fullPath)
})

test('the console is inert without the license: its 404 also leads to the login', async () => {
  const router = newRouter()
  worker.use(handleStaffMeGet(() => problem(404, { detail: 'not found' })))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe(consoleLoginRoute.fullPath)
})

test('the console opens for a signed-in staff member', async () => {
  const router = newRouter()
  worker.use(handleStaffMeGet({ body: { id: '1', email: 'ops@example.com' } }))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe('/console')
})

test('the OAuth consent screen sends a signed-out user to login and remembers where to return', async () => {
  const router = newRouter()
  serveWorkspaces('unauthorized')

  await router.navigate({
    to: oauthConsentRoute.to,
    search: {
      client_id: 'c',
      redirect_uri: 'https://x.test/cb',
      code_challenge: 'p',
      state: 's',
      scope: 'a',
    },
  })

  expect(router.state.location.pathname).toBe(loginRoute.to)
  expect(router.state.location.search).toMatchObject({
    redirect: expect.stringContaining(oauthConsentRoute.to),
  })
})
