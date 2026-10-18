import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteAuditGetRetention,
  handleSiteAuditList,
  handleSiteEventsList,
  handleSiteIntegrationsList,
  handleSiteInvitationsList,
  handleSiteMembershipsList,
  handleSiteSendingDomainsList,
  handleSiteSuppressionsList,
  handleSiteTokensList,
  handleSiteUserGetMe,
  handleSiteWebhooksList,
  handleSiteWorkspacesList,
  handleSiteWorkspacesUpdate,
} from '../../generated/site/msw.gen.ts'
import { Route as activityRoute } from '../../routes/workspaces/$slug/activity.tsx'
import { Route as settingsRoute } from '../../routes/workspaces/$slug/settings.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { SettingsPage } from './settings.tsx'

const SETTINGS_ROUTE = routeMount(settingsRoute, { slug: 'test' })

const workspace = {
  id: '1',
  name: 'Acme',
  slug: 'test',
  collectKey: 'omck_test_key',
  ingestKey: 'omik_test_key',
  postalAddress: '',
  accountLinkAvailable: false,
  holds: [],
  role: 'owner' as const,
  createdAt: '2026-01-01T00:00:00Z',
}

const emptyPage = { items: [], page: 1, pageSize: 20, totalItems: 0, totalPages: 0 }

// The settings page lists the workspaces, polls the events feed for the install status and
// embeds the other workspace sections, all empty here.
function serveWorkspace(totalItems = 0) {
  worker.use(
    handleSiteMembershipsList({ body: [] }),
    handleSiteUserGetMe({
      body: {
        id: '1',
        name: 'Me',
        email: 'me@example.com',
        emailVerified: true,
        createdAt: '2026-01-01T00:00:00Z',
      },
    }),
    handleSiteInvitationsList({ body: [] }),
    handleSiteAuditGetRetention({ body: { retentionDays: null } }),
    handleSiteAuditList({ body: { items: [] } }),
    handleSiteSendingDomainsList({ body: emptyPage }),
    handleSiteWebhooksList({ body: emptyPage }),
    handleSiteSuppressionsList({ body: emptyPage }),
    handleSiteWorkspacesList({ body: [workspace] }),
    handleSiteEventsList({ body: { items: [], page: 1, pageSize: 1, totalItems, totalPages: 0 } }),
    handleSiteTokensList({ body: [] }),
    handleSiteIntegrationsList({ body: [] }),
  )
}

test('renames the workspace and shows the tracking snippet and test command', async () => {
  const puts: string[] = []
  serveWorkspace()
  worker.use(
    handleSiteWorkspacesUpdate(async ({ request }) => {
      puts.push(await request.text())
      return HttpResponse.json({ ...workspace, name: 'Acme Inc' })
    }),
  )

  const { screen } = await renderWithRouter(<SettingsPage />, SETTINGS_ROUTE)

  // Tracking snippet and the curl test command both carry the collect key.
  await expect.element(screen.getByText(/omck_test_key/).first()).toBeInTheDocument()
  await expect.element(screen.getByText(/x-collect-key: omck_test_key/)).toBeInTheDocument()
  // No events yet → waiting state.
  await expect.element(screen.getByText('Waiting for the first event…')).toBeInTheDocument()

  const nameInput = screen.getByLabelText(/^Workspace name/)
  await expect.element(nameInput).toHaveValue('Acme')
  await nameInput.fill('Acme Inc')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => puts.length).toBeGreaterThan(0)
  expect(puts[0]).toContain('"name":"Acme Inc"')
})

test('shows the connected install status once events arrive', async () => {
  serveWorkspace(7)

  const { screen } = await renderWithRouter(<SettingsPage />, SETTINGS_ROUTE)

  await expect.element(screen.getByText(/Events are arriving/)).toBeInTheDocument()
})

test('the activity link opens the workspace activity feed', async () => {
  serveWorkspace()
  const { screen, navigate } = await renderWithRouter(<SettingsPage />, SETTINGS_ROUTE)

  await screen.getByRole('button', { name: 'Open activity feed' }).click()

  expect(navigate).toHaveBeenCalledWith({ to: activityRoute.to, params: { slug: 'test' } })
})

test('shows an error alert when the workspaces fail to load', async () => {
  worker.use(
    handleSiteWorkspacesList(() => problem(500, { detail: 'boom' })),
    handleSiteEventsList(() => problem(500, { detail: 'boom' })),
    handleSiteTokensList(() => problem(500, { detail: 'boom' })),
    handleSiteIntegrationsList(() => problem(500, { detail: 'boom' })),
  )
  const { screen } = await renderWithRouter(<SettingsPage />, SETTINGS_ROUTE)

  await expect.element(screen.getByRole('alert', { name: /Failed to load/ })).toBeInTheDocument()
})
