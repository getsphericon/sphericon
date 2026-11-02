import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource, SiteAutomationStep } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { emailIntegration, TIMESTAMPS } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, { workspaceId: 'TestWs01', automationId: '5' })

const FIRST: SiteAutomationStep = { type: 'email', subject: 'Hi', body: '<p>Hi</p>' }
const SECOND: SiteAutomationStep = { type: 'email', subject: 'Bye', body: '<p>Bye</p>' }

// The automation as stored with these steps in its draft.
function storedWith(steps: SiteAutomationStep[]): SiteAutomationResource {
  return {
    id: '5',
    name: 'Welcome flow',
    status: 'inactive',
    live: null,
    draft: { number: 1, triggerEvent: 'email.clicked', exitEvents: [], goalEvent: null, steps },
    ...TIMESTAMPS,
  }
}

const INTEGRATIONS = [
  emailIntegration('1', 'Primary SMTP', { isDefault: true }),
  emailIntegration('21', 'Mailgun relay'),
  emailIntegration('22', 'Text relay', { channel: 'sms' }),
]

// The canvas node at index (0 is the Trigger), once drawn.
function flowNode(index: number) {
  const node = document.querySelectorAll('.react-flow__node')[index]
  if (!node) {
    throw new Error('the canvas has not drawn the node yet')
  }
  return node
}

// Renders the editor over the stored steps, recording the body of every save, which the server
// stores and answers with.
async function editing(steps: SiteAutomationStep[]) {
  const bodies: { steps?: SiteAutomationStep[] }[] = []
  let stored = storedWith(steps)
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = await request.json()
      bodies.push(body)
      stored = storedWith(body.steps ?? steps)
      return HttpResponse.json(stored)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  return { screen, bodies }
}

// Opens the properties of the step node at index and returns its Integration picker.
async function integrationPickerOf(
  screen: Awaited<ReturnType<typeof editing>>['screen'],
  index: number,
) {
  await expect.poll(() => flowNode(index)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(index)).click()
  await screen.getByText('Properties').click()
  return screen.getByRole('combobox', { name: 'Integration' })
}

test('each email step picks its own Integration among the email ones, and saves it with the step', async () => {
  const { screen, bodies } = await editing([FIRST, SECOND])

  const picker = await integrationPickerOf(screen, 1)
  await expect.element(picker).toHaveValue('Workspace default')
  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Text relay' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Mailgun relay' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([[{ ...FIRST, integrationId: '21' }, SECOND]])
})

test('a step loaded with its Integration shows it, and saving sends it back on that step only', async () => {
  const { screen, bodies } = await editing([{ ...FIRST, integrationId: '21' }, SECOND])

  await expect.element(await integrationPickerOf(screen, 1)).toHaveValue('Mailgun relay')
  await expect.element(await integrationPickerOf(screen, 2)).toHaveValue('Workspace default')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([[{ ...FIRST, integrationId: '21' }, SECOND]])
})

test('choosing the default again saves the step without an Integration', async () => {
  const { screen, bodies } = await editing([{ ...FIRST, integrationId: '21' }])

  const picker = await integrationPickerOf(screen, 1)
  await picker.click()
  await screen.getByRole('option', { name: 'Workspace default' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies.map((body) => body.steps)).toEqual([[FIRST]])
})
