import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationPathInput,
  AutomationStep,
  AutomationStepInput,
  SiteAutomationResource,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import {
  emailIntegration,
  NO_AUTOMATION_REPORT,
  stepId,
  testId,
  TIMESTAMPS,
} from '../../test/payloads.ts'
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
  // The canvas shows the figures of the Automation's report: none here.
  worker.use(handleSiteAutomationReportsGet({ body: NO_AUTOMATION_REPORT }))
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

const STEP: AutomationStep = {
  id: stepId(11),
  type: 'telegram',
  text: 'Hi <b>there</b>',
  buttons: [],
  linkPreview: true,
  integrationId: testId(31),
}

// STEP as a save sends it: its defaults left out.
const SENT: AutomationStepInput = {
  id: stepId(11),
  type: 'telegram',
  text: 'Hi <b>there</b>',
  integrationId: testId(31),
}

const INTEGRATIONS = [
  emailIntegration(testId(1), 'Primary SMTP', { isDefault: true }),
  emailIntegration(testId(31), 'Acme bot', {
    channel: 'telegram',
    provider: 'telegram',
    config: { kind: 'telegram', botId: 7001, username: 'acme_bot' },
  }),
]

function stored(steps: AutomationStep[]): SiteAutomationResource {
  return {
    id: testId(5),
    name: 'Welcome flow',
    status: 'inactive',
    live: null,
    draft: {
      number: 1,
      trigger: { kind: 'event', action: 'telegram.bound', entryPolicy: 'once', cooldown: 3600 },
      exitEvents: [],
      goalEvent: null,
      steps: { id: stepId(1), steps },
    },
    revision: 1,
    ...TIMESTAMPS,
  }
}

test('a Telegram step round-trips: its bot and text load, an edit saves them back', async () => {
  const bodies: { steps?: AutomationPathInput }[] = []
  const current = stored([STEP])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(current)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await openNode(screen, 'Send Telegram message')
  const picker = screen.getByRole('combobox', { name: 'Integration' })
  await expect.element(picker).toHaveValue('Acme bot')
  await picker.click()
  await expect.element(screen.getByRole('option', { name: 'Primary SMTP' })).not.toBeInTheDocument()
  await screen.getByRole('option', { name: 'Acme bot' }).click()
  const text = screen.getByRole('textbox', { name: 'Message' })
  await expect.element(text).toHaveValue('Hi <b>there</b>')
  await expect.element(screen.getByText('15 / 4,096')).toBeVisible()
  await text.fill('Welcome')
  await expect.element(screen.getByText('7 / 4,096')).toBeVisible()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps?.steps))
    .toEqual([[{ ...SENT, text: 'Welcome' }]])
})

test('a Telegram text over the limit is flagged by the counter', async () => {
  worker.use(
    handleSiteAutomationsGet(() =>
      HttpResponse.json(
        stored([
          {
            id: stepId(11),
            type: 'telegram',
            text: 'x'.repeat(4100),
            buttons: [],
            linkPreview: true,
          },
        ]),
      ),
    ),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await openNode(screen, 'Send Telegram message')
  await expect.element(screen.getByText('4,100 / 4,096')).toBeVisible()
  await expect.element(screen.getByText(/it cannot go out until it is shorter/)).toBeVisible()
})

test('adding an image switches the counter to the caption limit, and buttons and the preview switch round-trip', async () => {
  const bodies: { steps?: AutomationPathInput }[] = []
  const current = stored([{ ...STEP, text: 'Hello {{ first_name }}' }])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(current)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await openNode(screen, 'Send Telegram message')
  await expect.element(screen.getByText('22 / 4,096')).toBeVisible()

  await screen.getByRole('textbox', { name: 'Image URL' }).fill('https://cdn.example.com/a.png')
  await expect.element(screen.getByText('22 / 1,024')).toBeVisible()
  const previewSwitch = screen.getByRole('switch', { name: 'Show a preview of links in the text' })
  await previewSwitch.click()
  await expect.element(previewSwitch).not.toBeChecked()
  await screen.getByRole('button', { name: 'Add a button' }).click()
  await screen.getByRole('textbox', { name: 'Button 1 label' }).fill('Open')
  await screen.getByRole('textbox', { name: 'Button 1 URL' }).fill('https://example.com/a')
  await expect.element(screen.getByTestId('telegram-preview')).toHaveTextContent('Hello AlexOpen')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps?.steps))
    .toEqual([
      [
        {
          ...SENT,
          text: 'Hello {{ first_name }}',
          imageUrl: 'https://cdn.example.com/a.png',
          buttons: [{ label: 'Open', url: 'https://example.com/a' }],
          linkPreview: false,
        },
      ],
    ])

  // Back from the server, the saved content loads into the editor again.
  await screen.getByRole('button', { name: 'Close panel' }).click()
  await openNode(screen, 'Send Telegram message')
  await expect.element(screen.getByRole('textbox', { name: 'Button 1 label' })).toHaveValue('Open')
})

test('the unsubscribe button of a Telegram step is switched on with a label and saved', async () => {
  const bodies: { steps?: AutomationPathInput }[] = []
  const current = stored([STEP])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(current)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await openNode(screen, 'Send Telegram message')
  await screen.getByText('Unsubscribe button', { exact: true }).click({ force: true })
  const label = screen.getByRole('textbox', { name: 'Button label' })
  await expect.element(label).toHaveValue('Unsubscribe')
  await label.fill('Stop messages')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps?.steps))
    .toEqual([[{ ...SENT, unsubscribeButton: 'Stop messages' }]])
})

test('a Telegram step with an unsubscribe button loads its label, and switching it off drops it', async () => {
  const bodies: { steps?: AutomationPathInput }[] = []
  const current = stored([{ ...STEP, unsubscribeButton: 'Stop' }])
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(current)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(current)
    }),
    handleSiteIntegrationsList({ body: INTEGRATIONS }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await openNode(screen, 'Send Telegram message')
  await expect.element(screen.getByRole('textbox', { name: 'Button label' })).toHaveValue('Stop')
  await screen.getByText('Unsubscribe button', { exact: true }).click({ force: true })
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies.map((body) => body.steps?.steps)).toEqual([[SENT]])
})
