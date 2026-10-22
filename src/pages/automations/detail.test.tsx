import { expect, test } from 'vitest'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteEnrollmentsList,
} from '../../generated/site/msw.gen.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { page, TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationDetailPage } from './detail.tsx'

const MOUNT = routeMount(automationsDetailRoute, { workspaceId: 'TestWs01', automationId: '1' })

test('shows the automation and the contacts enrolled in it', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
        name: 'Welcome series',
        status: 'active',
        triggerEvent: 'contact.created',
        steps: [],
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: { opens: 0, clicks: 0, delivered: 0, bounced: 0, complained: 0, links: [] },
    }),
    handleSiteEnrollmentsList({
      body: page(
        [
          {
            id: '5',
            automationId: '1',
            contactId: '42',
            status: 'completed',
            currentStep: 1,
            ...TIMESTAMPS,
          },
        ],
        1,
      ),
    }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Welcome series')).toBeInTheDocument()
  await expect.element(screen.getByText('42')).toBeInTheDocument()
  await expect.element(screen.getByText('Completed')).toBeInTheDocument()
})

test('shows the opens, the clicks and the clicked links of the automation', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
        name: 'Welcome series',
        status: 'active',
        triggerEvent: 'contact.created',
        steps: [],
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: {
        opens: 12,
        clicks: 9,
        delivered: 31,
        bounced: 4,
        complained: 5,
        links: [
          { url: 'https://dest.test/a', clicks: 7 },
          { url: 'https://dest.test/b', clicks: 2 },
        ],
      },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Opens', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('12', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Delivered', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('31', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Bounced', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('4', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Complained', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('5', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('https://dest.test/a')).toBeInTheDocument()
  await expect.element(screen.getByText('https://dest.test/b')).toBeInTheDocument()
})

test('marks a draft automation as a draft', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        id: '1',
        name: 'Unfinished flow',
        status: 'draft',
        triggerEvent: 'contact.created',
        steps: [],
        ...TIMESTAMPS,
      },
    }),
    handleSiteAutomationReportsGet({
      body: { opens: 0, clicks: 0, delivered: 0, bounced: 0, complained: 0, links: [] },
    }),
    handleSiteEnrollmentsList({ body: page([], 0) }),
  )

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Unfinished flow')).toBeInTheDocument()
  await expect.element(screen.getByText('Draft', { exact: true })).toBeInTheDocument()
})

test('shows an error alert when the automation fails to load', async () => {
  worker.use(handleSiteAutomationsGet(() => problem(500)))

  const { screen } = await renderWithRouter(<AutomationDetailPage />, MOUNT)

  await expect.element(screen.getByText('Failed to load automations')).toBeInTheDocument()
})
