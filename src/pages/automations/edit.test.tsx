import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'
import { page } from 'vitest/browser'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsUpdate,
  handleSiteEnrollmentsList,
  handleSiteIntegrationsList,
  handleSiteTagsList,
  handleSiteWorkspacesList,
} from '../../generated/site/msw.gen.ts'
import type { SiteAutomationResource, AutomationVersion } from '../../generated/site/types.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { stepId, testId, workspace } from '../../test/payloads.ts'
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

// The page also asks for the failed Enrollments and the Integrations to pick from: none unless a
// test says otherwise; and for the Workspace, whose postal address an email step shows when missing.
beforeEach(() => {
  worker.use(
    handleSiteWorkspacesList({ body: [workspace()] }),
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
          trigger: { kind: 'event', action: 'email.clicked' },
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
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()
  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger Event' }))
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
    trigger: { kind: 'event', action: 'email.clicked' },
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
        trigger: { kind: 'event', action: 'email.clicked' },
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
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('course_completed')
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toMatchObject([{ trigger: { kind: 'event', action: 'course_completed' } }])
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
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('order.completed')
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
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('button', { name: /Rule/ }).first().click()
  // The panel covers the canvas: each node is picked by a click dispatched on it.
  flowNode(1).dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await expect.element(screen.getByText('Trigger Filter')).not.toBeInTheDocument()
  flowNode(0).dispatchEvent(new MouseEvent('click', { bubbles: true }))
  await expect.element(screen.getByText('Trigger Filter')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect
    .poll(() => bodies)
    .toEqual([expect.objectContaining({ trigger: { kind: 'event', action: 'email.clicked' } })])
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
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('combobox', { name: 'Trigger Event' }).fill('contact.tagged')
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
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('button', { name: 'Add goal event' }).click()
  await screen.getByRole('combobox', { name: 'Event', exact: true }).fill('order.closed')
  await screen.getByRole('button', { name: /Rule/ }).last().click()
  await screen.getByLabelText('Event property name').fill('status')
  await screen.getByTestId('value-editor').fill('paid')
  // The value is left by a click elsewhere, as a person leaves it; the goal's editor lengthens the
  // panel over the bar, so the Save is clicked by an event dispatched on it.
  await screen.getByText('Goal Event').click()
  screen
    .getByRole('button', { name: 'Save' })
    .element()
    .dispatchEvent(new MouseEvent('click', { bubbles: true }))

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
    trigger: { kind: 'event', action: 'email.clicked' },
    exitEvents: [],
    goalEvent: null,
    steps: { id: stepId(1), steps },
  }
}

// An End (ADR 0124) is drawn, but this canvas cannot edit around one yet: a version holding one
// opens read-only, with nothing to save that could drop it.
test('a version holding an End opens read-only, drawing the End and offering no save', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        draft: withSteps([
          { id: stepId(11), type: 'wait', seconds: 60 },
          { id: stepId(12), type: 'end' },
        ]),
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)

  await expect.poll(() => flowNode(2)).toBeInstanceOf(Element)
  await expect
    .element(screen.getByText('This version ends a path with an End step', { exact: false }))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
  await expect.element(screen.getByLabelText('Name', { exact: false })).toBeDisabled()
  await expect.element(screen.getByText('End', { exact: true }).first()).toBeInTheDocument()
  await page.elementLocator(flowNode(2)).click()
  await screen.getByText('Properties').click()
  await expect
    .element(screen.getByText('This version ends a path with an End step', { exact: false }))
    .toBeInTheDocument()
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
        trigger: { kind: 'event', action: 'email.clicked' },
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

// The canvas node at `index` (0 is the Trigger); it throws until the canvas has drawn it, which
// poll retries.
function flowNode(index: number) {
  const node = document.querySelectorAll('.react-flow__node')[index]
  if (!node) {
    throw new Error('the canvas has not drawn the node yet')
  }
  return node
}

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
  await expect.poll(() => flowNode(1)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(1)).click()
  await screen.getByText('Properties').click()
  await screen.getByRole('button', { name: 'Edit message' }).click()

  await expect.element(screen.getByText('Hello')).toBeInTheDocument()
})

test('a refused document shows the server reason naming the step', async () => {
  worker.use(
    handleSiteAutomationsGet({ body: STORED }),
    handleSiteAutomationsUpdate(() =>
      problem(422, {
        title: 'Unprocessable Entity',
        detail: 'automations: invalid step: step 2: heading level 9 is not allowed',
      }),
    ),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await screen.getByRole('button', { name: 'Save' }).click()

  await expect.element(screen.getByText('Failed to save automation')).toBeInTheDocument()
  await expect.element(screen.getByText(/step 2: heading level 9/)).toBeInTheDocument()
})

test('the builder edits the draft and shows it pending above the live version', async () => {
  worker.use(
    handleSiteAutomationsGet({
      body: {
        ...STORED,
        status: 'active',
        live: {
          number: 1,
          trigger: { kind: 'event', action: 'email.opened' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
        },
        draft: {
          number: 2,
          trigger: { kind: 'event', action: 'email.clicked' },
          exitEvents: [],
          goalEvent: null,
          steps: { id: stepId(1), steps: [] },
        },
      },
    }),
  )

  const { screen } = await renderWithRouter(<AutomationEditPage />, EDIT_ROUTE)
  await expect.element(screen.getByText('Unpublished changes')).toBeInTheDocument()
  await expect.poll(() => flowNode(0)).toBeInstanceOf(Element)
  await page.elementLocator(flowNode(0)).click()
  await screen.getByText('Properties').click()

  await expect
    .element(screen.getByRole('combobox', { name: 'Trigger Event' }))
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
