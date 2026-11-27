import { HttpResponse } from 'msw'
import { useRef } from 'react'
import { beforeEach, expect, test } from 'vitest'

import {
  handleSiteAutomationsDryRunHttpStep,
  handleSiteContactsList,
} from '../../generated/site/msw.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { stepId, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { automationTags } from '../../utils/mergeTags.ts'
import type { StepDrafts } from './definition.ts'
import { HttpForm } from './StepForms.tsx'

const EDIT_ROUTE = routeMount(automationsEditRoute, {
  workspaceId: 'TestWs01',
  automationId: testId(5),
})

type StepData = StepDrafts['http']

const NEW: StepData = { stored: false, url: '', headers: [], body: '{}', secret: '' }

// The HTTP step's form as the side panel renders it, over the draft the flow holds.
function StepField({ data, onChange }: { data: StepData; onChange?: (data: StepData) => void }) {
  const validatorRef = useRef<(() => boolean) | null>(null)
  return (
    <HttpForm
      step={{ id: stepId(11), kind: 'http', draft: data }}
      onEdit={(step) => onChange?.(step.draft)}
      validatorRef={validatorRef}
      bound={automationTags([])}
    />
  )
}

const contacts = handleSiteContactsList({
  body: {
    items: [
      {
        id: testId(11),
        email: 'alice@example.com',
        tags: [],
        revision: 1,
        createdAt: '2026-01-01T00:00:00Z',
        updatedAt: '2026-01-01T00:00:00Z',
      },
    ],
    page: 1,
    pageSize: 50,
    totalItems: 1,
    totalPages: 1,
  },
})

// The dry run under the body offers the Workspace's Contacts.
beforeEach(() => {
  worker.use(contacts)
})

test('headers are rows: added, edited and removed, at most five', async () => {
  let latest = NEW
  const { screen } = await renderWithRouter(
    <StepField
      data={latest}
      onChange={(next) => {
        latest = next
      }}
    />,
    EDIT_ROUTE,
  )

  await screen.getByRole('button', { name: 'Add header' }).click()
  await screen.getByLabelText('Header name').fill('Authorization')
  await screen.getByLabelText('Header value').fill('Bearer x')
  await expect.poll(() => latest.headers).toEqual([{ name: 'Authorization', value: 'Bearer x' }])

  const addHeader = screen.getByRole('button', { name: 'Add header' })
  await addHeader.click()
  await addHeader.click()
  await addHeader.click()
  await addHeader.click()
  await expect.element(screen.getByRole('button', { name: 'Add header' })).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Remove header' }).first().click()
  await expect.poll(() => latest.headers).toHaveLength(4)
})

test('a header of a stored step shows its name and keeps its value when left empty', async () => {
  const { screen } = await renderWithRouter(
    <StepField data={{ ...NEW, stored: true, headers: [{ name: 'X-Key' }] }} />,
    EDIT_ROUTE,
  )

  await expect.element(screen.getByLabelText('Header name')).toHaveValue('X-Key')
  await expect.element(screen.getByText('Stored. Leave empty to keep it.')).toBeInTheDocument()
})

test('a dry run renders the body being edited for a contact, sending nothing', async () => {
  const asked: unknown[] = []
  worker.use(
    contacts,
    handleSiteAutomationsDryRunHttpStep(async ({ request }) => {
      asked.push(await request.json())
      return HttpResponse.json({ body: '{"email":"alice@example.com"}' })
    }),
  )
  const { screen } = await renderWithRouter(
    <StepField data={{ ...NEW, body: '{"email":"{{ email }}"}' }} />,
    EDIT_ROUTE,
  )

  await screen.getByRole('combobox', { name: 'Render for contact' }).click()
  await screen.getByRole('option', { name: 'alice@example.com' }).click()
  await screen.getByRole('button', { name: 'Dry run' }).click()

  await expect.element(screen.getByText('{"email":"alice@example.com"}')).toBeInTheDocument()
  expect(asked).toEqual([{ body: '{"email":"{{ email }}"}', contactId: testId(11) }])
})

test('a dry run shows why the body is refused', async () => {
  worker.use(
    contacts,
    handleSiteAutomationsDryRunHttpStep({
      body: { errorKind: 'invalid_body', detail: 'the body is not valid JSON' },
    }),
  )
  const { screen } = await renderWithRouter(<StepField data={{ ...NEW, body: '{' }} />, EDIT_ROUTE)

  await screen.getByRole('combobox', { name: 'Render for contact' }).click()
  await screen.getByRole('option', { name: 'alice@example.com' }).click()
  await screen.getByRole('button', { name: 'Dry run' }).click()

  await expect.element(screen.getByText('The body is not valid')).toBeInTheDocument()
  await expect.element(screen.getByText('the body is not valid JSON')).toBeInTheDocument()
})

test('the body is edited as text', async () => {
  let latest = { ...NEW, body: '' }
  worker.use(contacts)
  const { screen } = await renderWithRouter(
    <StepField
      data={latest}
      onChange={(next) => {
        latest = next
      }}
    />,
    EDIT_ROUTE,
  )

  await screen.getByRole('textbox', { name: 'JSON body' }).fill('{"a":1}')

  await expect.poll(() => latest.body).toBe('{"a":1}')
})

test('a contact without an email is offered by its id, and a failed dry run says so', async () => {
  worker.use(
    handleSiteContactsList({
      body: {
        items: [
          {
            id: testId(12),
            tags: [],
            revision: 1,
            createdAt: '2026-01-01T00:00:00Z',
            updatedAt: '2026-01-01T00:00:00Z',
          },
        ],
        page: 1,
        pageSize: 50,
        totalItems: 1,
        totalPages: 1,
      },
    }),
    handleSiteAutomationsDryRunHttpStep(() => problem(500, { detail: 'renderer down' })),
  )
  const { screen } = await renderWithRouter(<StepField data={NEW} />, EDIT_ROUTE)

  await screen.getByRole('combobox', { name: 'Render for contact' }).click()
  await screen.getByRole('option', { name: testId(12) }).click()
  await screen.getByRole('button', { name: 'Dry run' }).click()

  await expect.element(screen.getByText('Dry run failed')).toBeInTheDocument()
  await expect.element(screen.getByText('renderer down')).toBeInTheDocument()
})
