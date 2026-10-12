import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteOAuthDecide,
  handleSiteOAuthDescribe,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { OAuthConsent } from './consent.tsx'

const request = {
  clientId: 'fixture-client',
  redirectUri: 'https://connector.example/callback',
  codeChallenge: 'a'.repeat(43),
  state: 'xyz',
  scope: 'contacts:read',
}

const workspace = (slug: string, name: string) => ({
  id: slug,
  name,
  slug,
  collectKey: 'omck',
  ingestKey: 'omik',
  postalAddress: '',
  accountLinkAvailable: false,
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
})

function serve(
  workspaces: ReturnType<typeof workspace>[],
  decide: Parameters<typeof handleSiteOAuthDecide>[0],
) {
  worker.use(
    handleSiteWorkspacesList({ body: workspaces }),
    handleSiteOAuthDescribe({
      body: {
        clientName: 'Fixture Connector',
        redirectUri: request.redirectUri,
        scopes: ['contacts:read'],
        sendScopes: [],
      },
    }),
    handleSiteOAuthDecide(decide),
  )
}

test('the decision targets the workspace the user picked', async () => {
  const slugs: string[] = []
  serve([workspace('acme', 'Acme'), workspace('beta', 'Beta')], async ({ request: req }) => {
    slugs.push((await req.json()).workspaceSlug)
    return HttpResponse.json({ redirectUrl: '#done' })
  })
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByRole('combobox', { name: 'Workspace' }).click()
  await screen.getByRole('option', { name: 'Beta' }).click()
  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect.poll(() => slugs).toEqual(['beta'])
})

test('without a workspace the user cannot approve or deny', async () => {
  let decided = false
  serve([], () => {
    decided = true
    return HttpResponse.json({ redirectUrl: '#done' })
  })
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await expect.element(screen.getByRole('button', { name: 'Allow access' })).toBeDisabled()
  await screen.getByRole('button', { name: 'Deny' }).click()

  expect(decided).toBe(false)
})

test('an unexpected failure is toasted with the API detail', async () => {
  serve([workspace('acme', 'Acme')], () => problem(500, { detail: 'the code store is down' }))
  const { screen } = await renderWithRouter(<OAuthConsent request={request} />)

  await screen.getByRole('button', { name: 'Allow access' }).click()

  await expect.element(screen.getByText('Authorization failed')).toBeInTheDocument()
  await expect.element(screen.getByText('the code store is down')).toBeInTheDocument()
})
