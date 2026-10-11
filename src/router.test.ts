import { expect, test } from 'vitest'

import { handleOperatorMeGet } from './generated/operator/msw.gen.ts'
import { handleSiteWorkspacesList } from './generated/site/msw.gen.ts'
import {
  consoleHomeRoute,
  consoleLoginRoute,
  indexRoute,
  loginRoute,
  oauthConsentRoute,
  router,
  workspaceRoute,
} from './router.tsx'
import { problem } from './test/problem.ts'
import { worker } from './test/worker.ts'

const workspace = (slug: string) => ({
  id: slug,
  name: slug,
  slug,
  collectKey: 'omck',
  ingestKey: 'omik',
  postalAddress: '',
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
})

// The guards only ever call the workspaces list, so one stub per scenario is enough.
function serveWorkspaces(slugs: string[] | 'unauthorized') {
  worker.use(
    slugs === 'unauthorized'
      ? handleSiteWorkspacesList(() => problem(401, { detail: 'unauthorized' }))
      : handleSiteWorkspacesList({ body: slugs.map(workspace) }),
  )
}

test('the index route sends a signed-in user to their first workspace', async () => {
  serveWorkspaces(['acme', 'other'])

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(workspaceRoute.fullPath.replace('$slug', 'acme'))
})

test('the index route sends a user without workspaces to login', async () => {
  serveWorkspaces([])

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(loginRoute.to)
})

test('the index route sends an unauthorized user to login', async () => {
  serveWorkspaces('unauthorized')

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(loginRoute.to)
})

test('an unknown workspace slug falls back to the first workspace', async () => {
  serveWorkspaces(['acme'])

  await router.navigate({ to: workspaceRoute.to, params: { slug: 'nope' } })

  expect(router.state.location.pathname).toBe(workspaceRoute.fullPath.replace('$slug', 'acme'))
})

test('an unknown workspace slug with no workspaces goes to login', async () => {
  serveWorkspaces([])

  await router.navigate({ to: workspaceRoute.to, params: { slug: 'nope' } })

  expect(router.state.location.pathname).toBe(loginRoute.to)
})

test('the console sends a visitor without an Operator session to the console login', async () => {
  worker.use(handleOperatorMeGet(() => problem(401, { detail: 'unauthorized' })))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe(consoleLoginRoute.fullPath)
})

test('the console is inert without the license: its 404 also leads to the login', async () => {
  worker.use(handleOperatorMeGet(() => problem(404, { detail: 'not found' })))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe(consoleLoginRoute.fullPath)
})

test('the console opens for a signed-in Operator', async () => {
  worker.use(handleOperatorMeGet({ body: { id: '1', email: 'ops@example.com' } }))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe('/console')
})

test('the OAuth consent screen sends a signed-out user to login and remembers where to return', async () => {
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
