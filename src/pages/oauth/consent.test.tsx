import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteOAuthDecide,
  handleSiteOAuthDescribe,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SendScope } from '../../generated/site/types.gen.ts'
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
  accountLinkAvailable: false,
  mcpUrl: 'https://sphericon.test',
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const describeHandler = (
  sendScopes: SendScope[],
  extra: { clientHost?: string; localRedirect?: boolean } = {},
) =>
  handleSiteOAuthDescribe({
    body: {
      clientName: 'Fixture Connector',
      redirectUri: request.redirectUri,
      localRedirect: extra.localRedirect ?? false,
      ...(extra.clientHost ? { clientHost: extra.clientHost } : {}),
      scopes: ['contacts:read'],
      sendScopes,
    },
  })

function mockConsentApi(onDecide: (body: unknown) => void) {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler(['emails:send']),
    handleSiteOAuthDecide(async ({ request: req }) => {
      onDecide(await req.json())
      // Stay on the page: the test only inspects the decision that was sent.
      return HttpResponse.json({ redirectUrl: '#done' })
    }),
  )
}

test('shows the client and its permissions, with sending off by default', async () => {
  mockConsentApi(() => {})
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
  mockConsentApi(() => {})
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
    handleSiteOAuthDescribe(() => problem(400, { detail: 'bad' })),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect.element(screen.getByText(/authorization request is invalid/)).toBeInTheDocument()
})

test('a plain member who is forbidden sees why, not a generic error', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace] }),
    describeHandler([]),
    handleSiteOAuthDecide(() =>
      problem(403, { detail: 'only owners and admins can connect an application' }),
    ),
  )
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect
    .element(screen.getByText(/Only workspace owners and admins can connect an application/))
    .toBeInTheDocument()
})
