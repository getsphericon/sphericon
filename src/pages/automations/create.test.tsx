import { HttpResponse } from 'msw'
import { expect, test, vi } from 'vitest'

import { handleSiteAutomationsCreate } from '../../generated/site/msw.gen.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { AutomationCreateModal } from './create.tsx'

const WORKSPACE_ID = { workspaceId: 'TestWs01' }
const CREATE_ROUTE = routeMount(automationsRoute, WORKSPACE_ID)
const onClose = vi.fn<() => void>()
const NOW = '2026-01-01T00:00:00Z'

test('creating an automation sends the trimmed name and the default trigger, then opens the builder', async () => {
  const bodies: unknown[] = []
  worker.use(
    handleSiteAutomationsCreate(async ({ request }) => {
      bodies.push(await request.json())
      return HttpResponse.json(
        {
          id: '7',
          name: 'Welcome',
          status: 'inactive',
          triggerEvent: 'contact.created',
          steps: [],
          createdAt: NOW,
          updatedAt: NOW,
        },
        { status: 201 },
      )
    }),
  )

  const { screen, navigate } = await renderWithRouter(
    <AutomationCreateModal opened onClose={onClose} />,
    CREATE_ROUTE,
  )
  await expect.element(screen.getByText('New automation')).toBeInTheDocument()

  await screen.getByLabelText('Name', { exact: false }).fill('  Welcome  ')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.poll(() => bodies).toEqual([{ name: 'Welcome', triggerEvent: 'contact.created' }])
  await expect.element(screen.getByText('Automation created')).toBeInTheDocument()
  await expect.poll(() => navigate.mock.calls.length).toBe(1)
  expect(navigate.mock.calls[0]?.[0]).toMatchObject({
    params: { workspaceId: 'TestWs01', automationId: '7' },
  })
})

test('a refused create shows an error toast', async () => {
  worker.use(handleSiteAutomationsCreate(() => problem(500)))

  const { screen } = await renderWithRouter(
    <AutomationCreateModal opened onClose={onClose} />,
    CREATE_ROUTE,
  )

  await screen.getByLabelText('Name', { exact: false }).fill('Opens')
  await screen.getByRole('button', { name: 'Continue' }).click()

  await expect.element(screen.getByText('Failed to save automation')).toBeInTheDocument()
})

test('Cancel closes the modal', async () => {
  const { screen } = await renderWithRouter(
    <AutomationCreateModal opened onClose={onClose} />,
    CREATE_ROUTE,
  )

  await screen.getByRole('button', { name: 'Cancel' }).click()

  expect(onClose).toHaveBeenCalledOnce()
})
