import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteWorkspacesGetRevenueSettings,
  handleSiteWorkspacesList,
  handleSiteWorkspacesUpdateRevenueSettings,
} from '../../../generated/site/msw.gen.ts'
import type {
  Permission,
  SiteRevenueSettingsResource,
  SiteWorkspaceResource,
} from '../../../generated/site/types.gen.ts'
import { problem } from '../../../test/problem.ts'
import { renderWithRouter } from '../../../test/renderWithRouter.tsx'
import { worker } from '../../../test/worker.ts'
import { RevenueSettingsSection } from './RevenueSettingsSection.tsx'

const workspace = (permissions: Permission[]): SiteWorkspaceResource => ({
  id: 'TestWs01',
  name: 'Acme',
  collectKey: 'ck',
  postalAddress: '',
  timezone: 'UTC',
  accountLinkAvailable: false,
  permissions,
  mcpUrl: 'https://sphericon.test',
  trackerUrl: 'https://t.sphericon.test',
  holds: [],
  sendCap: null,
  role: 'member',
  createdAt: '2026-01-01T00:00:00Z',
})

const unset: SiteRevenueSettingsResource = { reportingCurrency: null, attributionWindowDays: 5 }

test('revenue:configure chooses the reporting currency and the attribution window', async () => {
  const bodies: string[] = []
  worker.use(
    handleSiteWorkspacesList({ body: [workspace(['revenue:configure'])] }),
    handleSiteWorkspacesGetRevenueSettings({ body: unset }),
    handleSiteWorkspacesUpdateRevenueSettings(async ({ request }) => {
      bodies.push(await request.text())
      return HttpResponse.json({ reportingCurrency: 'EUR', attributionWindowDays: 14 })
    }),
  )
  const { screen } = await renderWithRouter(
    <RevenueSettingsSection workspace={workspace(['revenue:configure'])} />,
  )

  const window = screen.getByRole('textbox', { name: /Attribution window/ })
  await expect.element(window).toHaveValue('5')
  await screen.getByRole('combobox', { name: 'Reporting currency' }).fill('EUR')
  await screen.getByRole('option', { name: 'EUR' }).click()
  await window.fill('14')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual(['{"reportingCurrency":"EUR","attributionWindowDays":14}'])
})

test('without revenue:configure both values show and do not change', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace([])] }),
    handleSiteWorkspacesGetRevenueSettings({
      body: { reportingCurrency: 'USD', attributionWindowDays: 9 },
    }),
  )
  const { screen } = await renderWithRouter(<RevenueSettingsSection workspace={workspace([])} />)

  const currency = screen.getByRole('combobox', { name: 'Reporting currency' })
  await expect.element(currency).toHaveValue('USD')
  await expect.element(currency).toBeDisabled()
  const window = screen.getByRole('textbox', { name: /Attribution window/ })
  await expect.element(window).toHaveValue('9')
  await expect.element(window).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})

test('says why when the server refuses the currency', async () => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace(['revenue:configure'])] }),
    handleSiteWorkspacesGetRevenueSettings({ body: unset }),
    handleSiteWorkspacesUpdateRevenueSettings(() =>
      problem(422, { title: 'unknown currency: use an ISO 4217 code such as EUR' }),
    ),
  )
  const { screen } = await renderWithRouter(
    <RevenueSettingsSection workspace={workspace(['revenue:configure'])} />,
  )

  await screen.getByRole('combobox', { name: 'Reporting currency' }).fill('EUR')
  await screen.getByRole('option', { name: 'EUR' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText(/unknown currency/)).toBeInTheDocument()
})
