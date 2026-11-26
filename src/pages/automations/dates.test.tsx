import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page, userEvent } from 'vitest/browser'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsUpdate,
  handleSiteCustomFieldsList,
  handleSiteEnrollmentsList,
  handleSiteEventsActions,
  handleSiteEventsPropertyKeys,
  handleSiteIntegrationsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationStep,
  AutomationTrigger,
  SiteAutomationResource,
  SiteCustomFieldResource,
  UpdateAutomationInput,
} from '../../generated/site/types.gen.ts'
import { zWeekday } from '../../generated/site/zod.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import {
  NO_AUTOMATION_REPORT,
  page as pageOf,
  stepId,
  TIMESTAMPS,
  testId,
  workspace,
} from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

// The canvas edits the date Trigger and the Wait until (ADR 0126, #1613's editor).

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})
const UUID = /^[\da-f]{8}-[\da-f]{4}-4[\da-f]{3}-[89ab][\da-f]{3}-[\da-f]{12}$/
const anyId = expect.stringMatching(UUID)

const TAG: AutomationStep = { id: stepId(12), type: 'apply_tag', tag: 'vip' }
const EVENT: AutomationTrigger = {
  kind: 'event',
  action: 'order.paid',
  entryPolicy: 'once',
  cooldown: 3600,
}

// A Custom field of the catalogue, of the type given.
function customField({
  id,
  ...field
}: { id: number } & Pick<
  SiteCustomFieldResource,
  'key' | 'name' | 'type'
>): SiteCustomFieldResource {
  return {
    id: testId(id),
    ...field,
    conditionTests: { text: [], number: [], moment: [], withinLast: false, presence: [] },
    revision: 1,
    ...TIMESTAMPS,
  }
}

const CATALOGUE = [
  customField({ id: 1, key: 'trial_ends_at', name: 'Trial ends', type: 'datetime' }),
  customField({ id: 2, key: 'plan', name: 'Plan', type: 'string' }),
  customField({ id: 3, key: 'signed_up_at', name: 'Signed up', type: 'datetime' }),
]

beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
})

// Renders the editor over the stored Trigger and steps, in a Workspace whose time zone is
// Europe/Berlin, recording the body of every save.
async function editing(trigger: AutomationTrigger, steps: AutomationStep[]) {
  const bodies: UpdateAutomationInput[] = []
  const stored: SiteAutomationResource = {
    id: testId(5),
    name: 'Birthday flow',
    status: 'inactive',
    live: null,
    draft: { number: 1, trigger, exitEvents: [], goalEvent: null, steps: { id: stepId(1), steps } },
    revision: 1,
    ...TIMESTAMPS,
  }
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationReportsGet({ body: NO_AUTOMATION_REPORT }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(stored)
    }),
    handleSiteEventsActions({
      body: { actions: [], eventsKeptDays: 400, messageActions: [], keptActions: [] },
    }),
    handleSiteEventsPropertyKeys({ body: { keys: [] } }),
    handleSiteCustomFieldsList({ body: pageOf(CATALOGUE) }),
    handleSiteIntegrationsList({ body: [] }),
    handleSiteWorkspacesList({ body: [workspace({ timezone: 'Europe/Berlin' })] }),
    handleSiteEnrollmentsList({ body: pageOf([]) }),
  )
  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  return { screen, saved: () => bodies }
}

type Screen = Awaited<ReturnType<typeof editing>>['screen']

// The button of the canvas node whose kind is `label`.
function nodeButton(screen: Screen, label: string) {
  return screen.getByRole('button', { name: new RegExp(`^${label}`) }).first()
}

function panel(screen: Screen, title: string) {
  return screen.getByRole('region', { name: title })
}

async function save(screen: Screen) {
  await screen.getByRole('button', { name: 'Save' }).click()
}

// The input of the Select labelled `label` in the panel: its listbox has the label too.
function select(screen: Screen, region: string, label: string) {
  return panel(screen, region).getByLabelText(label, { exact: true }).first()
}

// Picks the option of the Select, once it is listed: a Custom field's, once the catalogue is read.
async function pick(screen: Screen, input: ReturnType<typeof select>, option: string) {
  await input.click()
  const listed = screen.getByRole('option', { name: option })
  await expect.element(listed).toBeVisible()
  await listed.click()
}

test('switching the Trigger to a date hides the Entry policy, cooldown and key, and saves the date alone', async () => {
  const { screen, saved } = await editing({ ...EVENT, entryPolicy: 'again', cooldown: 7200 }, [TAG])

  await nodeButton(screen, 'Trigger').click()
  const trigger = panel(screen, 'Trigger')
  await expect.element(select(screen, 'Trigger', 'Enroll a contact')).toBeVisible()
  await expect.element(trigger.getByLabelText('Cooldown, hours')).toBeVisible()
  await trigger.getByText('A date').click()

  await expect.element(select(screen, 'Trigger', 'Enroll a contact')).not.toBeInTheDocument()
  await expect.element(trigger.getByLabelText('Cooldown, hours')).not.toBeInTheDocument()
  await expect.element(trigger.getByLabelText('Enrollment key')).not.toBeInTheDocument()
  await expect.element(trigger.getByLabelText('Trigger event')).not.toBeInTheDocument()
  // The exit and goal events are either kind's.
  await expect.element(trigger.getByRole('button', { name: 'Add exit event' })).toBeVisible()

  await pick(screen, select(screen, 'Trigger', 'Date'), 'Trial ends')
  await trigger.getByRole('textbox', { name: 'Days' }).fill('3')
  await pick(screen, select(screen, 'Trigger', 'Before or after'), 'Before the date')
  await trigger.getByLabelText('Local time').fill('10:00')
  await save(screen)

  await expect.poll(saved).toMatchObject([
    {
      trigger: {
        kind: 'date',
        field: { kind: 'customField', key: 'trial_ends_at' },
        recurrence: 'yearly',
        offsetDays: -3,
        localTime: '10:00',
      },
      steps: { id: stepId(1), steps: [TAG] },
    },
  ])
  expect(saved()[0]?.trigger).not.toHaveProperty('entryPolicy')
})

test("the date's field picker lists the birthday and only the catalogue's datetime fields", async () => {
  const { screen } = await editing(EVENT, [])

  await nodeButton(screen, 'Trigger').click()
  await panel(screen, 'Trigger').getByText('A date').click()
  await select(screen, 'Trigger', 'Date').click()

  await expect.element(screen.getByRole('option', { name: 'Birthday' })).toBeVisible()
  await expect.element(screen.getByRole('option', { name: 'Trial ends' })).toBeVisible()
  await expect.element(screen.getByRole('option', { name: 'Signed up' })).toBeVisible()
  await expect.element(screen.getByRole('option', { name: 'Plan' })).not.toBeInTheDocument()
})

test('the local time names the Workspace time zone a contact without one is read in', async () => {
  const { screen } = await editing(EVENT, [])

  await nodeButton(screen, 'Trigger').click()
  await panel(screen, 'Trigger').getByText('A date').click()

  await expect
    .element(
      screen.getByText(
        "Each contact's own time, or Europe/Berlin for a contact without a time zone.",
      ),
    )
    .toBeVisible()
})

test('a birthday only repeats every year', async () => {
  const { screen } = await editing(EVENT, [])

  await nodeButton(screen, 'Trigger').click()
  const trigger = panel(screen, 'Trigger')
  await trigger.getByText('A date').click()
  await trigger.getByText('Once').click()
  await pick(screen, select(screen, 'Trigger', 'Date'), 'Birthday')

  await expect.element(trigger.getByRole('radio', { name: 'Once' })).toBeDisabled()
  await expect.element(trigger.getByRole('radio', { name: 'Every year' })).toBeChecked()
})

test('a Wait until inserted on an edge, set to the next weekday at 9:00, is saved', async () => {
  const { screen, saved } = await editing(EVENT, [TAG])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait until' }).click()
  const waitUntil = panel(screen, 'Wait until')
  await waitUntil.getByLabelText('Local time').fill('09:00')
  await waitUntil.getByText('Monday', { exact: true }).click()
  await waitUntil.getByText('Tuesday', { exact: true }).click()
  await waitUntil.getByText('Wednesday', { exact: true }).click()
  await waitUntil.getByText('Thursday', { exact: true }).click()
  await waitUntil.getByText('Friday', { exact: true }).click()
  await save(screen)

  await expect
    .poll(() => saved().map((body) => body.steps?.steps))
    .toEqual([
      [
        {
          id: anyId,
          type: 'wait_until',
          until: {
            form: 'time_of_day',
            localTime: '09:00',
            // Monday to Friday, in the contract's order.
            weekdays: zWeekday.options.slice(0, 5),
          },
        },
        TAG,
      ],
    ])
})

test("a Wait until on a date is edited in the date Trigger's form", async () => {
  const { screen, saved } = await editing(EVENT, [])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait until' }).click()
  const waitUntil = panel(screen, 'Wait until')
  await waitUntil.getByText('A date').click()

  // The fields and the note of the Trigger's date form.
  await pick(screen, select(screen, 'Wait until', 'Date'), 'Signed up')
  await expect.element(waitUntil.getByText('Repeats')).toBeVisible()
  await waitUntil.getByText('Once').click()
  await waitUntil.getByRole('textbox', { name: 'Days' }).fill('7')
  await waitUntil.getByLabelText('Local time').fill('18:30')
  await expect
    .element(waitUntil.getByText(/or Europe\/Berlin for a contact without a time zone/))
    .toBeVisible()
  await save(screen)

  await expect
    .poll(() => saved().map((body) => body.steps?.steps))
    .toEqual([
      [
        {
          id: anyId,
          type: 'wait_until',
          until: {
            form: 'date',
            field: { kind: 'customField', key: 'signed_up_at' },
            recurrence: 'once',
            offsetDays: 7,
            localTime: '18:30',
          },
        },
      ],
    ])
})

const DATE_TRIGGER: AutomationTrigger = {
  kind: 'date',
  field: { kind: 'birthday' },
  recurrence: 'yearly',
  offsetDays: -3,
  localTime: '10:00',
}
const WAITS: AutomationStep[] = [
  {
    id: stepId(21),
    type: 'wait_until',
    until: { form: 'time_of_day', localTime: '09:00', weekdays: ['monday', 'friday'] },
  },
  {
    id: stepId(22),
    type: 'wait_until',
    until: {
      form: 'date',
      field: { kind: 'customField', key: 'trial_ends_at' },
      recurrence: 'once',
      offsetDays: 0,
      localTime: '08:00',
    },
  },
]

test('a version holding a date Trigger and Wait until Steps opens editable and saves them as stored', async () => {
  const { screen, saved } = await editing(DATE_TRIGGER, WAITS)

  await nodeButton(screen, 'Trigger').click()
  await expect.element(panel(screen, 'Trigger').getByLabelText('Local time')).toHaveValue('10:00')
  await nodeButton(screen, 'Wait until').click()
  await expect
    .element(panel(screen, 'Wait until').getByLabelText('Local time'))
    .toHaveValue('09:00')
  await screen
    .getByRole('button', { name: /^Wait until/ })
    .nth(1)
    .click()
  await expect
    .element(panel(screen, 'Wait until').getByLabelText('Local time'))
    .toHaveValue('08:00')
  await expect.element(screen.getByRole('button', { name: 'Save' })).toBeEnabled()
  await save(screen)

  await expect
    .poll(saved)
    .toMatchObject([{ trigger: DATE_TRIGGER, steps: { id: stepId(1), steps: WAITS } }])
})

test("a Wait until's error shows on its node and in its properties, and stops the Save", async () => {
  const { screen, saved } = await editing(EVENT, [])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait until' }).click()
  await save(screen)

  await expect.element(screen.getByText('Local time is required.')).toBeVisible()
  await expect
    .element(panel(screen, 'Wait until').getByText('This field is required'))
    .toBeVisible()
  expect(saved()).toEqual([])
})

test("a Wait until's and a date Trigger's errors show on their node and in their properties once a field is left", async () => {
  const { screen, saved } = await editing(EVENT, [TAG])

  await screen.getByRole('button', { name: 'Add a step here' }).first().click()
  await screen.getByRole('menuitem', { name: 'Wait until' }).click()
  await panel(screen, 'Wait until').getByLabelText('Local time').click()
  // Back out of it: a Tab moves on within its hour and minute.
  await userEvent.tab({ shift: true })

  await expect.element(screen.getByText('Local time is required.')).toBeVisible()
  await expect
    .element(panel(screen, 'Wait until').getByText('This field is required'))
    .toBeVisible()

  await nodeButton(screen, 'Trigger').click()
  await panel(screen, 'Trigger').getByText('A date').click()
  await select(screen, 'Trigger', 'Date').click()
  await userEvent.keyboard('{Escape}')
  await userEvent.tab()

  await expect.element(screen.getByText('Date is required.')).toBeVisible()
  await expect
    .element(panel(screen, 'Trigger').getByText('This field is required').first())
    .toBeVisible()
  expect(saved()).toEqual([])
})

test('a Save stopped on a date Trigger whose panel is closed opens it, showing the error', async () => {
  const { screen, saved } = await editing(EVENT, [TAG])

  await nodeButton(screen, 'Trigger').click()
  await panel(screen, 'Trigger').getByText('A date').click()
  await screen.getByRole('button', { name: 'Close panel' }).click()
  await save(screen)

  await expect.element(screen.getByText('Date is required.')).toBeVisible()
  await expect
    .element(panel(screen, 'Trigger').getByText('This field is required').first())
    .toBeVisible()
  expect(saved()).toEqual([])
})

// The server refuses a Custom field the catalogue holds with another type since the editor read
// it; the refusal names the Step and is placed at its location in the request (ADR 0126).
test("a refused save shows the error on the Wait until's node and in its properties", async () => {
  const refusal = `invalid step: step ${stepId(22)} (wait_until): the Custom field "trial_ends_at" is a string, not a datetime`
  const { screen } = await editing(EVENT, [TAG, ...WAITS.slice(1)])
  worker.use(
    handleSiteAutomationsUpdate(() =>
      problem(422, {
        title: 'Unprocessable Entity',
        detail: refusal,
        errors: { 'steps.steps.1': [refusal] },
      }),
    ),
  )

  await save(screen)

  const properties = panel(screen, 'Wait until')
  await expect.element(properties.getByText(refusal)).toBeVisible()
  // On the Wait until's node, and in its panel.
  expect(screen.getByText(refusal).elements()).toHaveLength(2)
  // Titled in the panel, and no toast besides.
  await expect.element(properties.getByText('Failed to save automation')).toBeVisible()
  expect(screen.getByText('Failed to save automation').elements()).toHaveLength(1)
})

test("a refused save shows the error on the date Trigger's node and in its properties", async () => {
  const refusal = 'invalid trigger: the Workspace has no Custom field "renews_at"'
  const { screen } = await editing(
    { ...DATE_TRIGGER, field: { kind: 'customField', key: 'trial_ends_at' } },
    [TAG],
  )
  worker.use(
    handleSiteAutomationsUpdate(() =>
      problem(422, {
        title: 'Unprocessable Entity',
        detail: refusal,
        errors: { trigger: [refusal] },
      }),
    ),
  )

  await save(screen)

  const properties = panel(screen, 'Trigger')
  await expect.element(properties.getByText(refusal)).toBeVisible()
  expect(screen.getByText(refusal).elements()).toHaveLength(2)
  await expect.element(properties.getByText('Failed to save automation')).toBeVisible()
  expect(screen.getByText('Failed to save automation').elements()).toHaveLength(1)
})

// A refusal of the request's shape lists each field it refused under one general detail: each node
// shows its own.
test('a refusal of two nodes shows each its own error, on its node and in its properties', async () => {
  const triggerError = 'localTime: must match "^([01][0-9]|2[0-3]):[0-5][0-9]$"'
  const waitError = 'offsetDays: must be at most 365'
  const { screen } = await editing(DATE_TRIGGER, [TAG, ...WAITS.slice(1)])
  worker.use(
    handleSiteAutomationsUpdate(() =>
      problem(422, {
        title: 'Unprocessable Entity',
        detail: 'The request is not valid.',
        errors: {
          'trigger.localTime': [triggerError],
          'steps.steps.1.until.offsetDays': [waitError],
        },
      }),
    ),
  )

  await save(screen)

  // The first refused node is opened.
  await expect.element(panel(screen, 'Trigger').getByText(triggerError)).toBeVisible()
  expect(screen.getByText(triggerError).elements()).toHaveLength(2)
  expect(screen.getByText(waitError).elements()).toHaveLength(1)
  await expect.element(screen.getByText('The request is not valid.')).not.toBeInTheDocument()

  await nodeButton(screen, 'Wait until').click()
  await expect.element(panel(screen, 'Wait until').getByText(waitError)).toBeVisible()
  expect(screen.getByText(waitError).elements()).toHaveLength(2)
})
