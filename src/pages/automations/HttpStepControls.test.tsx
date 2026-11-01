import { JsonForms } from '@workflowbuilder/sdk'
import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'
import * as z from 'zod'

import {
  handleSiteAutomationsDryRunHttpStep,
  handleSiteContactsList,
} from '../../generated/site/msw.gen.ts'
import { Route as automationsEditRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/edit.tsx'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { headersOf } from './definition.ts'
import { HTTP_BODY_FORMAT, HTTP_HEADERS_FORMAT } from './httpFormat.ts'
import { httpBodyRenderer, httpHeadersRenderer } from './httpRenderer.ts'

const EDIT_ROUTE = routeMount(automationsEditRoute, { workspaceId: 'TestWs01', automationId: '5' })

const schema = {
  type: 'object',
  properties: {
    stepId: { type: 'string' },
    headers: { type: 'string', format: HTTP_HEADERS_FORMAT },
    body: { type: 'string', format: HTTP_BODY_FORMAT },
  },
}

const stepData = z.object({ stepId: z.string(), headers: z.string(), body: z.string() })
type StepData = z.infer<typeof stepData>

// One field of the HTTP step's form, as the properties panel renders it: the app's control
// under the SDK's form, with the data the node holds.
function StepField({
  field,
  data,
  onChange,
}: {
  field: 'headers' | 'body'
  data: StepData
  onChange?: (data: StepData) => void
}) {
  return (
    <JsonForms
      schema={schema}
      uischema={{ type: 'Control', scope: `#/properties/${field}` }}
      data={data}
      renderers={[httpHeadersRenderer, httpBodyRenderer]}
      onChange={({ data: next }) => onChange?.(stepData.parse(next))}
    />
  )
}

test('headers are rows: added, edited and removed, at most five', async () => {
  let latest: StepData = { stepId: '', headers: '[]', body: '{}' }
  const { screen } = await renderWithRouter(
    <StepField
      field="headers"
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
  await expect
    .poll(() => headersOf(latest.headers))
    .toEqual([{ name: 'Authorization', value: 'Bearer x' }])

  for (let added = 1; added < 5; added++) {
    await screen.getByRole('button', { name: 'Add header' }).click()
  }
  await expect.element(screen.getByRole('button', { name: 'Add header' })).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Remove header' }).first().click()
  await expect.poll(() => headersOf(latest.headers)).toHaveLength(4)
})

test('a saved header shows its name and keeps its value when left empty', async () => {
  const { screen } = await renderWithRouter(
    <StepField
      field="headers"
      data={{ stepId: '01J', headers: '[{"name":"X-Key"}]', body: '{}' }}
    />,
    EDIT_ROUTE,
  )

  await expect.element(screen.getByLabelText('Header name')).toHaveValue('X-Key')
  await expect.element(screen.getByText('Stored. Leave empty to keep it.')).toBeInTheDocument()
})

const contacts = handleSiteContactsList({
  body: {
    items: [
      {
        id: '11',
        email: 'alice@example.com',
        tags: [],
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
    <StepField
      field="body"
      data={{ stepId: '', headers: '[]', body: '{"email":"{{ email }}"}' }}
    />,
    EDIT_ROUTE,
  )

  await screen.getByRole('combobox', { name: 'Render for contact' }).click()
  await screen.getByRole('option', { name: 'alice@example.com' }).click()
  await screen.getByRole('button', { name: 'Dry run' }).click()

  await expect.element(screen.getByText('{"email":"alice@example.com"}')).toBeInTheDocument()
  expect(asked).toEqual([{ body: '{"email":"{{ email }}"}', contactId: '11' }])
})

test('a dry run shows why the body is refused', async () => {
  worker.use(
    contacts,
    handleSiteAutomationsDryRunHttpStep({
      body: { errorKind: 'invalid_body', detail: 'the body is not valid JSON' },
    }),
  )
  const { screen } = await renderWithRouter(
    <StepField field="body" data={{ stepId: '', headers: '[]', body: '{' }} />,
    EDIT_ROUTE,
  )

  await screen.getByRole('combobox', { name: 'Render for contact' }).click()
  await screen.getByRole('option', { name: 'alice@example.com' }).click()
  await screen.getByRole('button', { name: 'Dry run' }).click()

  await expect.element(screen.getByText('The body is not valid')).toBeInTheDocument()
  await expect.element(screen.getByText('the body is not valid JSON')).toBeInTheDocument()
})
