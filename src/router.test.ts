import { QueryClient } from '@tanstack/react-query'
import { expect, test } from 'vitest'

import { handleSiteUserGetMe, handleSiteWorkspacesList } from './generated/site/msw.gen.ts'
import { handleStaffMeGet } from './generated/staff/msw.gen.ts'
import { handleStaffWorkspacesList } from './generated/staffee/msw.gen.ts'
import { createAppRouter } from './router.ts'
import { Route as acceptanceRoute } from './routes/acceptance.tsx'
import { Route as consoleWorkspacesRoute } from './routes/console/_authed/_enterprise/workspaces.index.tsx'
import { Route as consoleHomeRoute } from './routes/console/_authed/index.tsx'
import { Route as consoleLoginRoute } from './routes/console/login.tsx'
import { Route as indexRoute } from './routes/index.tsx'
import { Route as loginRoute } from './routes/login.tsx'
import { Route as oauthConsentRoute } from './routes/oauth/consent.tsx'
import { Route as signupSurveyRoute } from './routes/signup-survey.tsx'
import { Route as workspaceRoute } from './routes/workspaces/$workspaceId/route.tsx'
import { Route as newWorkspaceRoute } from './routes/workspaces/new.tsx'
import { problem } from './test/problem.ts'
import { worker } from './test/worker.ts'

// A router with a cache of its own: the guards read the workspaces through it.
const newRouter = () =>
  createAppRouter(new QueryClient({ defaultOptions: { queries: { retry: false } } }))

const workspace = (workspaceId: string) => ({
  id: workspaceId,
  name: workspaceId,
  workspaceId,
  collectKey: 'omck',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
})

// The guards read the signed-in user and then the workspaces list. The setup signs a user in by
// default, so a signed-out visitor is a 401 on both: with only the list refused, the login page would
// see a signed-in user and send the visitor back to the app.
function serveWorkspaces(ids: string[] | 'unauthorized') {
  if (ids === 'unauthorized') {
    worker.use(
      handleSiteUserGetMe(() => problem(401, { detail: 'unauthorized' })),
      handleSiteWorkspacesList(() => problem(401, { detail: 'unauthorized' })),
    )
    return
  }
  worker.use(handleSiteWorkspacesList({ body: ids.map((workspaceId) => workspace(workspaceId)) }))
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

test('the console leads to the login on an Instance without the Staff sign-in, whose surface is a 404', async () => {
  const router = newRouter()
  worker.use(handleStaffMeGet(() => problem(404, { detail: 'not found' })))

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe(consoleLoginRoute.fullPath)
})

const signedInStaff = () => handleStaffMeGet({ body: { id: '1', email: 'ops@example.com' } })

test('the console opens for a signed-in staff member without the license', async () => {
  const router = newRouter()
  worker.use(
    signedInStaff(),
    handleStaffWorkspacesList(() => problem(404, { detail: 'not found' })),
  )

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe('/console')
})

test('the console sends an Enterprise section the server does not serve to the console home', async () => {
  const router = newRouter()
  worker.use(
    signedInStaff(),
    handleStaffWorkspacesList(() => problem(404, { detail: 'not found' })),
  )

  await router.navigate({ to: consoleWorkspacesRoute.to })

  expect(router.state.location.pathname).toBe('/console')
})

test('the console opens an Enterprise section the server serves', async () => {
  const router = newRouter()
  worker.use(
    signedInStaff(),
    handleStaffWorkspacesList({
      body: { items: [], page: 1, pageSize: 1, totalItems: 0, totalPages: 0 },
    }),
  )

  await router.navigate({ to: consoleWorkspacesRoute.to })

  expect(router.state.location.pathname).toBe(consoleWorkspacesRoute.to)
})

test('a console that serves the Enterprise sections opens on the Workspaces, as before the core home', async () => {
  const router = newRouter()
  worker.use(
    signedInStaff(),
    handleStaffWorkspacesList({
      body: { items: [], page: 1, pageSize: 1, totalItems: 0, totalPages: 0 },
    }),
  )

  await router.navigate({ to: consoleHomeRoute.to })

  expect(router.state.location.pathname).toBe(consoleWorkspacesRoute.to)
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

// The terms a User has not accepted at their current version (ADR 0062).
const termsPending = [
  {
    key: 'service-terms',
    title: 'Terms of service',
    kind: 'required_acceptance',
    version: '2026-02',
  },
] as const

const user = (signupSurveyDue: boolean, acceptancesPending: typeof termsPending | [] = []) => ({
  id: 'UserDefault01',
  name: 'Test User',
  email: 'user@example.test',
  emailVerified: true,
  signupSurveyDue,
  acceptancesPending: [...acceptancesPending],
  createdAt: '2026-01-01T00:00:00Z',
})

test('a user who owes the Sign-up survey is sent to it from the index route', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])
  worker.use(handleSiteUserGetMe({ body: user(true) }))

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(signupSurveyRoute.to)
})

test('a user who does not owe the Sign-up survey is never sent to it', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])
  worker.use(handleSiteUserGetMe({ body: user(false) }))

  await router.navigate({ to: signupSurveyRoute.to })

  expect(router.state.location.pathname).toBe(
    workspaceRoute.fullPath.replace('$workspaceId', 'AcmeWs01'),
  )
})

test('a user with an acceptance pending is sent to it before the Sign-up survey', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])
  worker.use(handleSiteUserGetMe({ body: user(true, termsPending) }))

  await router.navigate({ to: indexRoute.to })

  expect(router.state.location.pathname).toBe(acceptanceRoute.to)
})

test('the Sign-up survey sends a user with an acceptance pending to it', async () => {
  const router = newRouter()
  worker.use(handleSiteUserGetMe({ body: user(true, termsPending) }))

  await router.navigate({ to: signupSurveyRoute.to })

  expect(router.state.location.pathname).toBe(acceptanceRoute.to)
})

test('a user with nothing pending is sent on from the acceptance screen', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])
  worker.use(handleSiteUserGetMe({ body: user(false) }))

  await router.navigate({ to: acceptanceRoute.to })

  expect(router.state.location.pathname).toBe(
    workspaceRoute.fullPath.replace('$workspaceId', 'AcmeWs01'),
  )
})

// The authorization request of an MCP client, as /oauth/authorize hands it to the consent screen.
const consentSearch = {
  client_id: 'c',
  redirect_uri: 'https://x.test/cb',
  code_challenge: 'p',
  state: 's',
  scope: 'a',
}

test('the OAuth consent screen sends a user with an acceptance pending to it and remembers where to return', async () => {
  const router = newRouter()
  worker.use(handleSiteUserGetMe({ body: user(false, termsPending) }))

  await router.navigate({ to: oauthConsentRoute.to, search: consentSearch })

  expect(router.state.location.pathname).toBe(acceptanceRoute.to)
  expect(router.state.location.search).toMatchObject({
    redirect: expect.stringContaining(`${oauthConsentRoute.to}?client_id=c`),
  })
})

test('the acceptance screen resumes the page asked for once nothing is pending', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])
  worker.use(handleSiteUserGetMe({ body: user(false) }))

  await router.navigate({ to: acceptanceRoute.to, search: { redirect: newWorkspaceRoute.to } })

  expect(router.state.location.pathname).toBe(newWorkspaceRoute.to)
})

test('the acceptance screen drops a return path to another origin', async () => {
  const router = newRouter()
  serveWorkspaces(['AcmeWs01'])
  worker.use(handleSiteUserGetMe({ body: user(false) }))

  await router.navigate({ to: acceptanceRoute.to, search: { redirect: '//evil.test/x' } })

  expect(router.state.location.pathname).toBe(
    workspaceRoute.fullPath.replace('$workspaceId', 'AcmeWs01'),
  )
})

test('a signed-out visitor at the acceptance screen is sent to the login', async () => {
  const router = newRouter()
  serveWorkspaces('unauthorized')

  await router.navigate({ to: acceptanceRoute.to })

  expect(router.state.location.pathname).toBe(loginRoute.to)
})
