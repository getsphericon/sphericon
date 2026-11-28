import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationReportsGet,
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsUnknownPath,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteEventsActions,
  handleSiteEventsPropertyKeys,
  handleSiteIntegrationsList,
  handleSiteTagsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type {
  AutomationStep,
  AutomationVersion,
  SiteAutomationResource,
  UpdateAutomationInput,
} from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { NO_AUTOMATION_REPORT, stepId, testId, workspace } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationEditPage } from './edit.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})
const NOW = '2026-01-01T00:00:00Z'

// The page also asks for the failed Enrollments and the Integrations to pick from, and the Trigger
// card for the property keys of recent events of its action: none unless a test says otherwise;
// for the Workspace, whose postal address an email step shows when missing; and for the report,
// whose figures the canvas shows: none.
beforeEach(async () => {
  // The editor is a desktop tool: its side panel sits beside the canvas, not over it.
  await page.viewport(1280, 900)
  worker.use(
    handleSiteAutomationReportsGet({ body: NO_AUTOMATION_REPORT }),
    handleSiteWorkspacesList({ body: [workspace()] }),
    handleSiteEventsPropertyKeys({ body: { keys: [] } }),
    handleSiteEventsActions({
      body: { actions: [], eventsKeptDays: 400, messageActions: [], keptActions: [] },
    }),
    handleSiteEnrollmentsList({
      body: { items: [], page: 1, pageSize: 25, totalItems: 0, totalPages: 0 },
    }),
    handleSiteIntegrationsList({ body: [] }),
  )
})

test('loads the automation into the builder', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        revision: 1,
        id: testId(5),
        name: 'Welcome flow',
        status: 'inactive',
        live: null,
        draft: {
          number: 1,
          trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [{ id: stepId(11), type: 'wait', seconds: 60 }] },
        },
        createdAt: NOW,
        updatedAt: NOW,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Name', { exact: false })).toHaveValue('Welcome flow')
  await openNode(screen, 'Trigger')
  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger event' }))
    .toHaveValue('email.clicked')
})

test('shows an error alert when the automation fails to load', async () => {
  worker.use(handleSiteAutomationsGet(() => problem(404, 'Gone')))

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText('Failed to load automations').first()).toBeInTheDocument()
})

const STORED: SiteAutomationResource = {
  revision: 1,
  id: testId(5),
  name: 'Welcome flow',
  status: 'inactive',
  live: null,
  draft: {
    number: 1,
    trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
    steps: {
      id: stepId(1),
      steps: [
        { id: stepId(11), type: 'wait', seconds: 60 },
        { id: stepId(12), type: 'apply_tag', tag: 'vip' },
      ],
    },
  },
  createdAt: NOW,
  updatedAt: NOW,
}

test('saving from the builder sends the edited name, the trigger and the canvas steps, each with its id', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByLabelText('Name', { exact: false }).fill('  Renamed  ')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        revision: 1,
        name: 'Renamed',
        trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
        exitEvents: [],
        goalEvent: null,
        steps: {
          id: stepId(1),
          steps: [
            { id: stepId(11), type: 'wait', seconds: 60 },
            { id: stepId(12), type: 'apply_tag', tag: 'vip' },
          ],
        },
      },
    ])
  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
})

test('the Trigger card sets the Entry policy and, under again, the cooldown in hours, which the save sends in seconds', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await expect.element(screen.getByLabelText('Cooldown, hours')).not.toBeInTheDocument()
  await screen.getByRole('combobox', { name: 'Enroll a contact' }).click()
  await screen.getByRole('option', { name: 'Again after each enrollment ends' }).click()
  await screen.getByLabelText('Cooldown, hours').fill('24')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([{ trigger: { kind: 'event', entryPolicy: 'again', cooldown: 86_400 } }])
})

test('the Trigger card suggests the property keys of recent events of its action as the Enrollment key, and the save sends the one picked', async () => {
  const bodies: unknown[] = []
  const asked: (string | null)[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEventsPropertyKeys(({ request }) => {
      asked.push(new URL(request.url).searchParams.get('action'))
      return HttpResponse.json({ keys: ['order_id', 'total'] })
    }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('combobox', { name: 'Enrollment key' }).click()
  await screen.getByRole('option', { name: 'order_id' }).click()
  await screen.getByRole('button', { name: 'Save' }).click()

  expect(asked).toContain('email.clicked')
  await expect
    .poll(() => bodies)
    .toMatchObject([
      { trigger: { kind: 'event', action: 'email.clicked', enrollmentKey: 'order_id' } },
    ])
})

test('saving sends the trigger event typed on the canvas, a custom one included', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('combobox', { name: 'Trigger event' }).fill('course_completed')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([
      {
        trigger: { kind: 'event', action: 'course_completed', entryPolicy: 'once', cooldown: 3600 },
      },
    ])
})

test("saving sends the Trigger's filter edited on the canvas with the Event's property tests", async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('combobox', { name: 'Trigger event' }).fill('order.completed')
  await screen.getByRole('button', { name: /Rule/ }).first().click()
  await screen.getByLabelText('Event property name').fill('status')
  await screen.getByTestId('value-editor').fill('paid')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([
      {
        trigger: {
          kind: 'event',
          action: 'order.completed',
          filter: {
            kind: 'group',
            combinator: 'and',
            entries: [
              {
                kind: 'property',
                key: 'status',
                test: { kind: 'text', operator: 'eq', value: 'paid' },
              },
            ],
          },
        },
      },
    ])
})

test('a filter left half-typed is dropped when the Trigger is opened again, and the Save sends none', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('button', { name: /Rule/ }).first().click()
  await openNode(screen, 'Wait')
  await expect.element(screen.getByText('Trigger event')).not.toBeInTheDocument()
  await openNode(screen, 'Trigger')
  await expect.element(screen.getByLabelText('Event property name')).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      expect.objectContaining({
        trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
      }),
    ])
})

test("the filter of a Tag's Trigger offers the Workspace's Tags as the tag tested", async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteTagsList({
      body: {
        items: [{ id: testId(1), name: 'trial', createdAt: NOW }],
        page: 1,
        pageSize: 100,
        totalItems: 1,
        totalPages: 1,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('combobox', { name: 'Trigger event' }).fill('contact.tagged')
  await screen.getByRole('button', { name: /Rule/ }).first().click()
  await screen.getByLabelText('Event property name').fill('tag')
  await screen.getByRole('combobox', { name: 'Value' }).click()

  await expect.element(screen.getByRole('option', { name: 'trial' })).toBeVisible()
})

test('saving sends a goal event added on the canvas, an Event matcher with its filter', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('button', { name: 'Add goal event' }).click()
  await screen.getByRole('combobox', { name: 'Event', exact: true }).fill('order.closed')
  await screen.getByRole('button', { name: /Rule/ }).last().click()
  await screen.getByLabelText('Event property name').fill('status')
  await screen.getByTestId('value-editor').fill('paid')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([
      {
        exitEvents: [],
        goalEvent: {
          action: 'order.closed',
          filter: {
            kind: 'group',
            combinator: 'and',
            entries: [
              {
                kind: 'property',
                key: 'status',
                test: { kind: 'text', operator: 'eq', value: 'paid' },
              },
            ],
          },
        },
      },
    ])
})

// withSteps is STORED's draft with other steps.
function withSteps(steps: AutomationVersion['steps']['steps']): AutomationVersion {
  return {
    number: 1,
    trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
    steps: { id: stepId(1), steps },
  }
}

// A tree of every fork kind and an End (ADR 0124): a Branch with an unknown path, a Split nested in
// one of its paths, an End finishing `otherwise`.
const TREE: AutomationVersion['steps']['steps'] = [
  {
    id: stepId(20),
    type: 'branch',
    paths: [
      {
        id: stepId(21),
        condition: {
          kind: 'group',
          combinator: 'and',
          entries: [{ kind: 'event', action: 'email.opened', operator: 'atLeast', times: 1 }],
        },
        steps: [
          {
            id: stepId(22),
            type: 'split',
            paths: [
              {
                id: stepId(23),
                weight: 30,
                steps: [{ id: stepId(24), type: 'wait', seconds: 60 }],
              },
              { id: stepId(25), weight: 70, steps: [] },
            ],
          },
        ],
      },
    ],
    otherwise: { id: stepId(26), steps: [{ id: stepId(27), type: 'end' }] },
    unknown: { id: stepId(28), steps: [] },
  },
  { id: stepId(29), type: 'apply_tag', tag: 'done' },
]

// Branches, Splits and End are edited (ADR 0124): a version holding them opens editable, draws
// each, and a save sends every one of them back, nothing dropped.
test('a version holding forks and an End opens editable, and a save sends the whole tree back', async () => {
  const bodies: UpdateAutomationInput[] = []
  worker.use(
    handleSiteAutomationsGet({ body: { ...STORED, draft: withSteps(TREE) } }),
    handleSiteAutomationsUnknownPath({ body: { needed: true } }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.element(nodeButton(screen, 'Branch')).toBeInTheDocument()
  await expect.element(nodeButton(screen, 'Split')).toBeInTheDocument()
  await expect.element(nodeButton(screen, 'End')).toBeInTheDocument()
  await screen.getByLabelText('Name', { exact: false }).fill('Forked')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies.map((body) => body.steps))
    .toEqual([{ id: stepId(1), steps: TREE }])
})

// A Step of a kind newer than this editor is kept as stored: the version opens read-only, with
// nothing to save that could drop it, and Publish still publishes it.
test('a version holding a step of a kind the editor does not know opens read-only', async () => {
  // A kind the contract may gain, read as the server would send it.
  const newer: AutomationStep = JSON.parse(
    JSON.stringify({ id: stepId(12), type: 'wait_for_reply' }),
  )
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        draft: withSteps([{ id: stepId(11), type: 'wait', seconds: 60 }, newer]),
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(nodeButton(screen, 'wait_for_reply')).toBeInTheDocument()
  await expect
    .element(
      screen.getByText('This version holds a step of a kind this editor does not know', {
        exact: false,
      }),
    )
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await expect.element(screen.getByLabelText('Name', { exact: false })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Remove step' })).not.toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Add a step here' }))
    .not.toBeInTheDocument()
  await openNode(screen, 'Wait')
  await expect.element(screen.getByRole('textbox', { name: 'Wait (seconds)' })).toBeDisabled()
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
})

const AUTHORED_DOC = {
  type: 'doc',
  content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello' }] }],
}

test('saving an email step written in the editor sends its document and no body', async () => {
  const bodies: unknown[] = []
  const authored: SiteAutomationResource = {
    ...STORED,
    draft: withSteps([
      {
        id: stepId(11),
        type: 'email',
        subject: 'Hi',
        body: '<p>Hello</p>',
        doc: AUTHORED_DOC,
        editorVersion: '1.7.14',
      },
    ]),
  }
  worker.use(
    handleSiteAutomationsGet({ body: authored }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(authored)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([
      {
        revision: 1,
        name: 'Welcome flow',
        trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
        exitEvents: [],
        goalEvent: null,
        steps: {
          id: stepId(1),
          steps: [{ id: stepId(11), type: 'email', subject: 'Hi', doc: AUTHORED_DOC }],
        },
      },
    ])
})

test('a failed save from the builder shows an error toast', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Failed to save automation')).toBeInTheDocument()
})

test('shows no failed enrollments when none has a recorded reason', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEnrollmentsList({
      body: {
        items: [
          {
            id: testId(9),
            automationId: testId(5),
            contactId: testId(107),
            status: 'failed',
            ordinal: 1,
            awaited: [],
            createdAt: NOW,
            updatedAt: NOW,
          },
        ],
        page: 1,
        pageSize: 25,
        totalItems: 1,
        totalPages: 1,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Name', { exact: false })).toBeInTheDocument()
  await expect.element(screen.getByText('Failed enrollments')).not.toBeInTheDocument()
})

test('shows no failed enrollments when the list cannot load', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEnrollmentsList(() => problem(500)),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByLabelText('Name', { exact: false })).toBeInTheDocument()
  await expect.element(screen.getByText('Failed enrollments')).not.toBeInTheDocument()
})

test('lists the enrollments an HTTP step failed, with the reason and the response', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteEnrollmentsList({
      body: {
        items: [
          {
            id: testId(9),
            automationId: testId(5),
            contactId: testId(107),
            status: 'failed',
            ordinal: 1,
            awaited: [],
            failure: { kind: 'http_status', status: 503, excerpt: 'upstream unavailable' },
            createdAt: NOW,
            updatedAt: NOW,
          },
        ],
        page: 1,
        pageSize: 25,
        totalItems: 1,
        totalPages: 1,
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await screen.getByRole('button', { name: 'Failed enrollments (1)' }).click()

  await expect.element(screen.getByText('The receiver answered 503')).toBeInTheDocument()
  await expect.element(screen.getByText('upstream unavailable')).toBeInTheDocument()
})

test('an email step opens its document in the editor from the properties panel', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        draft: withSteps([
          { id: stepId(11), type: 'email', subject: 'Hi', body: '<p>Hello</p>', doc: AUTHORED_DOC },
        ]),
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Send email')
  await screen.getByRole('button', { name: 'Edit message' }).click()

  await expect.element(screen.getByText('Hello')).toBeInTheDocument()
})

// The server places its refusal of a Step at the Step's location in the request.
test("a refused document shows the server's reason on its step's node and in its properties", async () => {
  const refusal = `invalid step: step ${stepId(12)} (email): heading level 9 is not allowed`
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        draft: withSteps([
          { id: stepId(11), type: 'wait', seconds: 60 },
          { id: stepId(12), type: 'email', subject: 'Hi', body: '<h9>Hi</h9>' },
        ]),
      },
    }),
    handleSiteAutomationsUpdate(() =>
      problem(422, {
        title: 'Unprocessable Entity',
        detail: refusal,
        errors: { 'steps.steps.1': [refusal] },
      }),
    ),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  const properties = screen.getByRole('region', { name: 'Send email' })
  await expect.element(properties.getByText('Failed to save automation')).toBeVisible()
  await expect.element(properties.getByText(refusal)).toBeVisible()
  // On the email step's node, and in its panel.
  expect(screen.getByText(refusal).elements()).toHaveLength(2)
})

test('the builder edits the draft and shows it pending above the live version', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        status: 'active',
        live: {
          number: 1,
          trigger: { kind: 'event', action: 'email.opened', entryPolicy: 'once', cooldown: 3600 },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
        },
        draft: {
          number: 2,
          trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
        },
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
  await openNode(screen, 'Trigger')

  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger event' }))
    .toHaveValue('email.clicked')
})

test('Publish waits for unsaved changes to be saved', async () => {
  worker.use(handleSiteAutomationsGet({ body: STORED }))

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeEnabled()
  await screen.getByLabelText('Name', { exact: false }).fill('Renamed')

  await expect.element(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
})

test('Publish from the builder publishes the saved draft', async () => {
  const published: string[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsPublish(({ params }) => {
      published.push(params.id)
      return HttpResponse.json({ ...STORED, status: 'active', live: STORED.draft, draft: null })
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Publish' }).click()

  await expect.poll(() => published).toEqual([testId(5)])
  await expect.element(screen.getByText('Automation published')).toBeInTheDocument()
})

// An archived Automation is read-only until it is unarchived (ADR 0100): its edit URL opens its page,
// which has no Edit, instead of a builder whose Save would answer 409.
test('an archived automation opens its page instead of the builder', async () => {
  worker.use(handleSiteAutomationsGet({ body: { ...STORED, archivedAt: NOW } }))

  const { screen, navigate } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect
    .poll(() => navigate)
    .toHaveBeenCalledWith({
      to: automationsDetailRoute.to,
      params: { workspaceId: 'TestWs01', automationId: testId(5) },
      replace: true,
    })
  await expect.element(screen.getByRole('button', { name: 'Save' })).not.toBeInTheDocument()
})

test('saving sends the exit events added on the canvas, less the one removed', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(STORED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Trigger')
  await screen.getByRole('button', { name: 'Add exit event' }).click()
  await screen.getByRole('combobox', { name: 'Event', exact: true }).fill('order.cancelled')
  await screen.getByRole('button', { name: 'Add exit event' }).click()
  await screen.getByRole('combobox', { name: 'Event', exact: true }).last().fill('order.refunded')
  await screen.getByRole('button', { name: 'Remove' }).first().click()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => bodies).toMatchObject([{ exitEvents: [{ action: 'order.refunded' }] }])
})

type Screen = Awaited<ReturnType<typeof renderWithRouter>>['screen']

// The button of the canvas node whose kind is `label` ("Trigger", "Wait"): its kind's name, then
// what it says beside it.
function nodeButton(screen: Screen, label: string) {
  return screen.getByRole('button', { name: new RegExp(`^${label}`) }).first()
}

// Opens the node in the side panel.
async function openNode(screen: Screen, label: string) {
  await nodeButton(screen, label).click()
}

// The Workspace the builder edits in, with no postal address yet (ADR 0156).
const NO_ADDRESS = workspace({ postalAddress: '' })

test('an email step on the canvas shows the missing postal address with a link to the setting', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        draft: withSteps([{ id: stepId(11), type: 'email', subject: 'Hi', body: '<p>Hi</p>' }]),
      },
    }),
    handleSiteWorkspacesList({ body: [NO_ADDRESS] }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(screen.getByText(/This workspace has no postal address/)).toBeInTheDocument()
  await expect
    .element(screen.getByRole('link', { name: 'Set the postal address' }))
    .toHaveAttribute('href', '/workspaces/TestWs01/settings')
})

test('a canvas without an email step shows no postal address notice', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteWorkspacesList({ body: [NO_ADDRESS] }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.element(nodeButton(screen, 'Wait')).toBeVisible()
  await expect
    .element(screen.getByText(/This workspace has no postal address/))
    .not.toBeInTheDocument()
})

// The lint's warnings of an email step (ADR 0040), as the server words them: never a refusal.
const missingAlt = {
  code: 'missing_alt' as const,
  message: 'Image has no alt text',
  element: '<img src="https://cdn.example.com/hero.png">',
}

const EMAILS: SiteAutomationResource = {
  ...STORED,
  draft: withSteps([
    {
      id: stepId(11),
      type: 'email',
      subject: 'Hi',
      body: '<img src="https://cdn.example.com/hero.png">',
    },
    { id: stepId(12), type: 'email', subject: 'Again', body: '<p>Again</p>' },
  ]),
}

const WARNED: SiteAutomationResource = {
  ...EMAILS,
  revision: 2,
  warnings: [{ stepId: stepId(11), warnings: [missingAlt] }],
}

test("a save shows each email step's warnings in its panel, with their message and element", async () => {
  let saved = false
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(saved ? WARNED : EMAILS)),
    handleSiteAutomationsUpdate(() => {
      saved = true
      return HttpResponse.json(WARNED)
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await openNode(screen, 'Send email')
  const properties = screen.getByRole('region', { name: 'Send email' })
  await expect.element(properties.getByLabelText('Subject')).toHaveValue('Hi')
  await expect.element(properties.getByText('Image has no alt text')).not.toBeInTheDocument()

  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Automation updated')).toBeInTheDocument()
  await expect.element(properties.getByText('Image has no alt text')).toBeVisible()
  await expect.element(properties.getByText(missingAlt.element)).toBeVisible()
})

test('an email step without warnings shows none in its panel', async () => {
  worker.use(handleSiteAutomationsGet({ body: WARNED }))

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen
    .getByRole('button', { name: /^Send email/ })
    .nth(1)
    .click()
  const properties = screen.getByRole('region', { name: 'Send email' })

  await expect.element(properties.getByLabelText('Subject')).toHaveValue('Again')
  await expect.element(properties.getByText('Image has no alt text')).not.toBeInTheDocument()
})

test('Publish shows the warnings of the email steps first, and still publishes', async () => {
  const published: string[] = []
  worker.use(
    handleSiteAutomationsGet({ body: WARNED }),
    handleSiteAutomationsPublish(({ params }) => {
      published.push(params.id)
      return HttpResponse.json({ ...WARNED, status: 'active', live: WARNED.draft, draft: null })
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Publish' }).click()

  const dialog = screen.getByRole('dialog', { name: 'Publish this automation?' })
  await expect.element(dialog.getByText('Send email: Hi')).toBeVisible()
  await expect.element(dialog.getByText('Image has no alt text')).toBeVisible()
  await expect.element(dialog.getByText(missingAlt.element)).toBeVisible()
  expect(published).toEqual([])
  await dialog.getByRole('button', { name: 'Publish' }).click()

  await expect.poll(() => published).toEqual([testId(5)])
  await expect.element(screen.getByText('Automation published')).toBeInTheDocument()
})
