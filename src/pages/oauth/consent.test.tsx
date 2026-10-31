import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteOAuthDecide,
  handleSiteOAuthDescribe,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import { SetupScope } from '../../generated/site/types.gen.ts'
import type { GrantableScope, Permission, SendScope } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { OAuthConsent } from './consent.tsx'

const request = {
  clientId: 'fixture-client',
  redirectUri: 'https://connector.example/callback',
  codeChallenge: 'a'.repeat(43),
  state: 'xyz',
  scope: 'contacts:read emails:send',
}

const workspace = {
  id: 'AcmeWs01',
  name: 'Acme',
  collectKey: 'omck_test_key',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions: [],
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  sendCap: null,
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const describeHandler = (
  sendScopes: SendScope[],
  extra: {
    clientHost?: string
    localRedirect?: boolean
    setupScopes?: SetupScope[]
    scopes?: GrantableScope[]
    requiredPermissions?: Record<string, Permission>
  } = {},
) =>
  handleSiteOAuthDescribe({
    body: {
      clientName: 'Fixture Connector',
      redirectUri: request.redirectUri,
      localRedirect: extra.localRedirect ?? false,
      ...(extra.clientHost ? { clientHost: extra.clientHost } : {}),
      scopes: extra.scopes ?? ['contacts:read'],
      sendScopes,
      setupScopes: extra.setupScopes ?? [],
      requiredPermissions: extra.requiredPermissions ?? {},
    },
  })

function mockConsentApi(onDecide: (body: unknown) => void, setupScopes: SetupScope[] = []) {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler(['emails:send'], { setupScopes }),
    handleSiteOAuthDecide(async ({ request: req }) => {
      onDecide(await req.json())
      // Stay on the page: the test only inspects the decision that was sent.
      return HttpResponse.json({ redirectUrl: '#done' })
    }),
  )
}

test('shows the client and its permissions, with sending off by default', async () => {
  mockConsentApi(vi.fn<() => void>())
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect
    .element(screen.getByText('Connect Fixture Connector to Sphericon'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('contacts:read', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByLabelText(/Also allow sending/)).not.toBeChecked()
})

test('names the host of a metadata-document client and warns about a localhost redirect', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler([], { clientHost: 'claude.example', localRedirect: true }),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect
    .element(screen.getByText('Requested by the app at claude.example'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('This app connects through localhost')).toBeInTheDocument()
})

test('shows no host and no localhost warning for an ordinary registered client', async () => {
  mockConsentApi(vi.fn<() => void>())
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect
    .element(screen.getByText('Connect Fixture Connector to Sphericon'))
    .toBeInTheDocument()
  expect(screen.getByText(/Requested by the app at/).elements()).toHaveLength(0)
  expect(screen.getByText('This app connects through localhost').elements()).toHaveLength(0)
})

test('approving without the send opt-in does not allow sending', async () => {
  const decisions: unknown[] = []
  mockConsentApi((body) => {
    decisions.push(body)
  })
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect.poll(() => decisions).toHaveLength(1)
  expect(decisions[0]).toMatchObject({
    clientId: 'fixture-client',
    workspaceId: 'AcmeWs01',
    approve: true,
    allowSend: false,
    state: 'xyz',
  })
})

test('ticking the send opt-in is carried to the decision', async () => {
  const decisions: unknown[] = []
  mockConsentApi((body) => {
    decisions.push(body)
  })
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByLabelText(/Also allow sending/).click()
  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect.poll(() => decisions).toHaveLength(1)
  expect(decisions[0]).toMatchObject({ approve: true, allowSend: true })
})

test('denying sends approve=false', async () => {
  const decisions: unknown[] = []
  mockConsentApi((body) => {
    decisions.push(body)
  })
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByRole('button', { name: 'Deny' }).click()

  await expect.poll(() => decisions).toHaveLength(1)
  expect(decisions[0]).toMatchObject({ approve: false, allowSend: false })
})

test('an invalid request shows an error instead of the form', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteOAuthDescribe(() => problem(422, { detail: 'bad' })),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect.element(screen.getByText(/authorization request is invalid/)).toBeInTheDocument()
})

test('a refused grant explains the role, not a generic error', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler([]),
    handleSiteOAuthDecide(() =>
      problem(403, { detail: 'the role lacks the permission', code: 'permission_required' }),
    ),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect
    .element(screen.getByText(/Your role in this workspace does not allow granting some/))
    .toBeInTheDocument()
})

test('offers and sends only the scopes the role in the chosen workspace may grant', async () => {
  const decisions: unknown[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler([], {
      scopes: ['contacts:read', 'webhooks:read'],
      setupScopes: ['sending_domains:write'],
      requiredPermissions: {
        'webhooks:read': 'webhooks:manage',
        'sending_domains:write': 'sending_domains:manage',
      },
    }),
    handleSiteOAuthDecide(async ({ request: req }) => {
      decisions.push(await req.json())
      return HttpResponse.json({ redirectUrl: '#done' })
    }),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect.element(screen.getByText('contacts:read', { exact: true })).toBeInTheDocument()
  expect(screen.getByText('webhooks:read', { exact: true }).elements()).toHaveLength(0)
  expect(screen.getByLabelText(/Also allow changing sending setup/).elements()).toHaveLength(0)
  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect.poll(() => decisions).toHaveLength(1)
  expect(decisions[0]).toMatchObject({ scope: 'contacts:read' })
})

test('a workspace where the role may grant none of the scopes cannot be approved', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler([], {
      scopes: ['webhooks:read'],
      requiredPermissions: { 'webhooks:read': 'webhooks:manage' },
    }),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect
    .element(screen.getByText(/does not allow granting any of the permissions/))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Allow access' })).toBeDisabled()
})

test('offers the setup opt-in only when the app asked for setup scopes, off by default', async () => {
  mockConsentApi(vi.fn<() => void>())
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)
  await expect
    .element(screen.getByText('Connect Fixture Connector to Sphericon'))
    .toBeInTheDocument()
  expect(screen.getByLabelText(/Also allow changing sending setup/).elements()).toHaveLength(0)

  worker.resetHandlers()
  mockConsentApi(vi.fn<() => void>(), ['sending_domains:write'])
  const asked = await renderWithRouter(<OAuthConsent request={request} />)
  await expect
    .element(asked.screen.getByLabelText(/Also allow changing sending setup/))
    .not.toBeChecked()
})

test('the setup opt-in is carried to the decision apart from the send opt-in', async () => {
  const decisions: unknown[] = []
  mockConsentApi((body) => {
    decisions.push(body)
  }, Object.values(SetupScope))
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByLabelText(/Also allow changing sending setup/).click()
  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect.poll(() => decisions).toHaveLength(1)
  expect(decisions[0]).toMatchObject({ approve: true, allowSetup: true, allowSend: false })
})

test('denying never carries the setup opt-in', async () => {
  const decisions: unknown[] = []
  mockConsentApi(
    (body) => {
      decisions.push(body)
    },
    ['sending_domains:write'],
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByLabelText(/Also allow changing sending setup/).click()
  await screen.getByRole('button', { name: 'Deny' }).click()

  await expect.poll(() => decisions).toHaveLength(1)
  expect(decisions[0]).toMatchObject({ approve: false, allowSetup: false })
})
