import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteBrandKitGet,
  handleSiteBrandKitRecreateStarterTemplates,
  handleSiteBrandKitSave,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteBrandKitResource, SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { Route as brandKitRoute } from '../../routes/workspaces/$workspaceId/brand-kit.tsx'
import { hex } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { BrandKitPage } from './page.tsx'

const workspace = (role: SiteWorkspaceResource['role']): SiteWorkspaceResource => ({
  id: 'AcmeWs01',
  name: 'Acme',

  collectKey: 'ck',
  postalAddress: '',
  accountLinkAvailable: false,
  holds: [],
  mcpUrl: 'https://example.com/mcp',
  role,
  createdAt: '2026-01-01T00:00:00Z',
})

const renderPage = (role: SiteWorkspaceResource['role']) => {
  worker.use(handleSiteWorkspacesList({ body: [workspace(role)] }))
  return renderWithRouter(<BrandKitPage />, routeMount(brandKitRoute, { workspaceId: 'AcmeWs01' }))
}

const KIT: SiteBrandKitResource = {
  saved: true,
  accentColor: hex('ff5500'),
  textColor: hex('102030'),
  backgroundColor: hex('f0f4f8'),
  headingFont: 'Georgia, serif',
  bodyFont: 'Verdana, sans-serif',
  logoUrl: 'https://cdn.example.com/logo.png',
  buttonTextColor: hex('fefefe'),
  buttonRadius: 12,
  footerText: 'Acme Inc',
  updatedAt: '2026-01-01T00:00:00Z',
}

const UNSAVED: SiteBrandKitResource = {
  ...KIT,
  saved: false,
  accentColor: hex('2563eb'),
  logoUrl: '',
  footerText: '',
  updatedAt: null,
}

test('an owner saves the Brand kit and the first save announces the starter templates', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteBrandKitGet({ body: UNSAVED }),
    handleSiteBrandKitSave(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(KIT)
    }),
  )
  const { screen } = await renderPage('owner')

  await expect.element(screen.getByText(/also creates four starter templates/)).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Recreate the starter templates' }))
    .not.toBeInTheDocument()
  await screen.getByLabelText('Footer').fill('Acme Inc')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        accentColor: hex('2563eb'),
        textColor: hex('102030'),
        backgroundColor: hex('f0f4f8'),
        headingFont: 'Georgia, serif',
        bodyFont: 'Verdana, sans-serif',
        logoUrl: '',
        buttonTextColor: hex('fefefe'),
        buttonRadius: 12,
        footerText: 'Acme Inc',
      },
    ])
})

test('a saved kit is recreated into starter templates only after a confirmation', async () => {
  let recreated = 0
  worker.use(
    handleSiteBrandKitGet({ body: KIT }),
    handleSiteBrandKitRecreateStarterTemplates(() => {
      recreated += 1
      return HttpResponse.json([], { status: 201 })
    }),
  )
  const { screen } = await renderPage('owner')

  await screen.getByRole('button', { name: 'Recreate the starter templates' }).click()
  await expect.element(screen.getByText(/Templates and messages that already exist/)).toBeVisible()
  await screen.getByRole('button', { name: 'Cancel' }).click()
  expect(recreated).toBe(0)

  await screen.getByRole('button', { name: 'Recreate the starter templates' }).click()
  await screen.getByRole('button', { name: 'Recreate', exact: true }).click()
  await expect.poll(() => recreated).toBe(1)
})

test.each(['member', 'admin'] as const)(
  '%s sees the Brand kit but cannot change it',
  async (role) => {
    worker.use(handleSiteBrandKitGet({ body: KIT }))
    const { screen } = await renderPage(role)

    await expect.element(screen.getByLabelText('Footer')).toHaveValue('Acme Inc')
    await expect.element(screen.getByLabelText('Footer')).toBeDisabled()
    await expect
      .element(screen.getByText('Only owners can change the Brand kit.'))
      .toBeInTheDocument()
    await expect.element(screen.getByRole('button', { name: 'Save' })).not.toBeInTheDocument()
  },
)

test('a refused value shows the reason the server gave', async () => {
  worker.use(
    handleSiteBrandKitGet({ body: KIT }),
    handleSiteBrandKitSave(() =>
      problem(422, { detail: 'accentColor must be a color like #1a2b3c' }),
    ),
  )
  const { screen } = await renderPage('owner')

  await expect.element(screen.getByLabelText('Footer')).toHaveValue('Acme Inc')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .element(screen.getByText('accentColor must be a color like #1a2b3c'))
    .toBeInTheDocument()
})
