import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationPathInput,
  AutomationStep,
  EmailStep,
  SiteAutomationResource,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { emailIntegration, stepId, TIMESTAMPS, testId, workspace } from '../../test/payloads.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})

beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
})

type Screen = Awaited<ReturnType<typeof renderWithRouter>>['screen']

// Opens the canvas node whose kind is `label` in the side panel; `index` picks among nodes of one
// kind, top to bottom.
async function openNode(screen: Screen, label: string, index = 0) {
  await screen
    .getByRole('button', { name: new RegExp(`^${label}`) })
    .nth(index)
    .click()
}

const FIRST: EmailStep = { id: stepId(11), type: 'email', subject: 'Hi', body: '<p>Hi</p>' }
const SECOND: EmailStep = { id: stepId(12), type: 'email', subject: 'Bye', body: '<p>Bye</p>' }

// The automation as stored with these steps in its draft.
function storedWith(steps: AutomationStep[]): SiteAutomationResource {
  return {
    id: testId(5),
    name: 'Welcome flow',
    status: 'inactive',
    live: null,
    draft: {
      number: 1,
      trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
      exitEvents: [],
      goalEvent: null,
      steps: { id: stepId(1), steps },
    },
    revision: 1,
    ...TIMESTAMPS,
  }
}

const INTEGRATIONS = [
  emailIntegration(testId(1), 'Primary SMTP', { isDefault: true }),
  emailIntegration(testId(21), 'Mailgun relay'),
  emailIntegration(testId(22), 'Text relay', { channel: 'sms' }),
]

// Renders the editor over the stored steps, recording the body of every save; the canvas keeps
// what it saved, so the server answers with the Automation as loaded.
async function editing(steps: AutomationStep[]) {
  const bodies: { steps?: AutomationPathInput }[] = []
  const stored = storedWith(steps)
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(stored)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  return { screen, bodies }
}

// Opens the email step at index (0 is the first) and returns its Integration picker.
async function integrationPickerOf(
  screen: Awaited<ReturnType<typeof editing>>['screen'],
  index: number,
) {
  await openNode(screen, 'Send email', index)
  return screen.getByRole('combobox', { name: 'Integration' })
}

test('each email step picks its own Integration among the email ones, and saves it with the step', async () => {
  const { screen, bodies } = await editing([FIRST, SECOND])

  const picker = await integrationPickerOf(screen, 0)
  await expect.element(picker).toHaveValue('Workspace default')
  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Text relay' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Mailgun relay' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps?.steps))
    .toEqual([[{ ...FIRST, integrationId: testId(21) }, SECOND]])
})

test('a step loaded with its Integration shows it, and saving sends it back on that step only', async () => {
  const { screen, bodies } = await editing([{ ...FIRST, integrationId: testId(21) }, SECOND])

  await expect.element(await integrationPickerOf(screen, 0)).toHaveValue('Mailgun relay')
  await expect.element(await integrationPickerOf(screen, 1)).toHaveValue('Workspace default')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps?.steps))
    .toEqual([[{ ...FIRST, integrationId: testId(21) }, SECOND]])
})

test('choosing the default again saves the step without an Integration', async () => {
  const { screen, bodies } = await editing([{ ...FIRST, integrationId: testId(21) }])

  const picker = await integrationPickerOf(screen, 0)
  await picker.click()
  await screen.getByRole('option', { name: 'Workspace default' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies.map((body) => body.steps?.steps)).toEqual([[FIRST]])
})
