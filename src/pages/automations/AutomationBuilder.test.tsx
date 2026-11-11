import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
} from '../../generated/site/msw.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

// The canvas can only drop branches when a node has several outgoing edges, which is
// impractical to draw in a test; report one dropped branch from the real conversion instead.
vi.mock(import('./definition.ts'), async (importOriginal) => {
  const original = await importOriginal()
  return {
    ...original,
    graphToSteps: (graph) => ({ ...original.graphToSteps(graph), dropped: 1 }),
  }
})

const EDIT_ROUTE = routeMount(automationsEditRoute, { workspaceId: 'TestWs01', automationId: '5' })
const NOW = '2026-01-01T00:00:00Z'
const DRAFT: SiteAutomationVersion = {
  number: 1,
  triggerEvent: 'email.clicked',
  exitEvents: [],
  goalEvent: null,
  steps: [{ type: 'wait', seconds: 60 }],
}
const STORED: SiteAutomationResource = {
  id: '5',
  name: 'Welcome flow',
  status: 'inactive',
  live: null,
  draft: DRAFT,
  createdAt: NOW,
  updatedAt: NOW,
}

test('the builder shows the SDK strings and the app strings inside it as text, not keys', async () => {
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(STORED)),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )

  const { screen, router } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  // The SDK's own panel, the app's bar, a properties control the app renders for the SDK, and
  // the dialog the leave guard opens from inside the canvas.
  await expect.element(screen.getByText('Nodes Library')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument()
  await screen.getByText('Trigger', { exact: true }).click()
  await expect
    .element(screen.getByText('The event that enrolls a contact into this automation.'))
    .toBeInTheDocument()
  await screen.getByRole('textbox', { name: 'Name' }).fill('Welcome flow, renamed')
  router.history.push('/workspaces/TestWs01/automations')
  await expect.element(screen.getByText('Leave without saving?')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Keep editing' })).toBeInTheDocument()
})

test('saving a graph whose branches cannot be represented warns that they were dropped', async () => {
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(STORED)),
    handleSiteAutomationsUpdate(() => HttpResponse.json(STORED)),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
  await expect.element(screen.getByText('Branches not supported yet')).toBeInTheDocument()
})
