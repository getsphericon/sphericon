import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleOperatorWorkspaceAuditList,
  handleOperatorWorkspacesGet,
  handleOperatorWorkspacesSuspend,
  handleOperatorWorkspacesUnsuspend,
} from '../generated/operator/msw.gen.ts'
import type { OperatorWorkspaceDetailResource } from '../generated/operator/types.gen.ts'
import { consoleWorkspaceRoute } from '../router.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { ConsoleWorkspacePage } from './workspace.tsx'

// Mounted at the route's id, not its fullPath: the console's guard route is pathless, so the
// two differ and `consoleWorkspaceRoute.useParams()` looks the match up by id.
const MOUNT = {
  path: consoleWorkspaceRoute.id,
  initialPath: consoleWorkspaceRoute.id.replace('$workspaceId', '5'),
}

const HOOLI: OperatorWorkspaceDetailResource = {
  id: '5',
  slug: 'hooli',
  name: 'Hooli',
  createdAt: '2026-01-01T00:00:00Z',
  suspension: {
    at: '2026-02-01T00:00:00Z',
    actor: { kind: 'operator', id: 'op-fixture' },
    reason: 'abuse report',
  },
  deliverability: { windowHours: 24, volumeFloor: 1000, sendVolume: 0, domains: [] },
}

const ABOVE_FLOOR = {
  sendingDomainId: '400',
  domain: 'mail.soylent.test',
  complaintRate: { numerator: 3, denominator: 1200, rate: 0.0025 },
  bounceRate: { numerator: 60, denominator: 1300, rate: 0.046 },
}

const BELOW_FLOOR = {
  sendingDomainId: '401',
  domain: 'promo.soylent.test',
  complaintRate: { numerator: 1, denominator: 7, rate: null },
  bounceRate: { numerator: 1, denominator: 8, rate: null },
}

// The page always reads the Audit log; tests that care override it.
beforeEach(() => {
  worker.use(handleOperatorWorkspaceAuditList({ body: { items: [] } }))
})

test('lists the audit log with an Operator shown as sphericon staff, and loads more', async () => {
  worker.use(
    handleOperatorWorkspacesGet({ body: HOOLI }),
    handleOperatorWorkspaceAuditList(({ request }) =>
      new URL(request.url).searchParams.get('cursor')
        ? HttpResponse.json({
            items: [
              {
                id: '1',
                occurredAt: '2026-01-01T00:00:00Z',
                actor: { kind: 'user', id: '9', name: 'Peter' },
                action: 'tag.create',
                target: { type: 'tag', id: '12', name: 'vip' },
              },
            ],
          })
        : HttpResponse.json({
            items: [
              {
                id: '2',
                occurredAt: '2026-02-01T00:00:00Z',
                actor: { kind: 'operator', name: 'sphericon staff' },
                action: 'workspace.suspend',
                target: { type: 'workspace', id: '5', name: 'Hooli' },
              },
            ],
            nextCursor: '2',
          }),
    ),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('sphericon staff')).toBeInTheDocument()
  await expect.element(screen.getByText('workspace.suspend')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Load more' }).click()
  await expect.element(screen.getByText('tag.create')).toBeInTheDocument()
  await expect.element(screen.getByText('Peter')).toBeInTheDocument()
})

test('an empty audit log says so', async () => {
  worker.use(handleOperatorWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('No recorded changes')).toBeInTheDocument()
})

test('shows the metadata and who suspended the workspace, and why', async () => {
  worker.use(handleOperatorWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByRole('heading', { name: 'Hooli' })).toBeInTheDocument()
  await expect.element(screen.getByText('hooli', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Operator op-fixture')).toBeInTheDocument()
  await expect.element(screen.getByText('abuse report')).toBeInTheDocument()
})

test('a workspace that is not suspended says so', async () => {
  worker.use(handleOperatorWorkspacesGet({ body: { ...HOOLI, suspension: null } }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('Sending is not suspended.')).toBeInTheDocument()
})

test('a non-operator actor shows its kind, not an id', async () => {
  worker.use(
    handleOperatorWorkspacesGet({
      body: {
        ...HOOLI,
        suspension: { at: '2026-02-01T00:00:00Z', actor: { kind: 'cli' }, reason: null },
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('Command line')).toBeInTheDocument()
  await expect.element(screen.getByText('No reason given')).toBeInTheDocument()
})

test('an unknown workspace shows the error', async () => {
  worker.use(handleOperatorWorkspacesGet(() => problem(404, { detail: 'workspace not found' })))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('workspace not found')).toBeInTheDocument()
})

test('shows the send volume and the rates with their counts per sending domain', async () => {
  worker.use(
    handleOperatorWorkspacesGet({
      body: {
        ...HOOLI,
        deliverability: {
          windowHours: 24,
          volumeFloor: 1000,
          sendVolume: 1308,
          domains: [ABOVE_FLOOR],
        },
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('mail.soylent.test')).toBeInTheDocument()
  await expect.element(screen.getByText('0.25%')).toBeInTheDocument()
  await expect.element(screen.getByText('3 / 1200')).toBeInTheDocument()
  await expect.element(screen.getByText('4.6%')).toBeInTheDocument()
  await expect.element(screen.getByText('1,308')).toBeInTheDocument()
  await expect.element(screen.getByText('Last 24 hours')).toBeInTheDocument()
})

test('a rate below the volume floor shows the notice and its counts, never a percentage', async () => {
  worker.use(
    handleOperatorWorkspacesGet({
      body: {
        ...HOOLI,
        deliverability: {
          windowHours: 24,
          volumeFloor: 1000,
          sendVolume: 8,
          domains: [BELOW_FLOOR],
        },
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('promo.soylent.test')).toBeInTheDocument()
  await expect
    .element(screen.getByText('Not enough data (fewer than 1000 sent)').first())
    .toBeInTheDocument()
  await expect.element(screen.getByText('1 / 8')).toBeInTheDocument()
  await expect.element(screen.getByText('1 / 7')).toBeInTheDocument()
})

test('a workspace with no sending domains says so', async () => {
  worker.use(handleOperatorWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('No sending domains.')).toBeInTheDocument()
})

test('suspending needs a reason, and sends it only after the confirm button', async () => {
  const suspended: unknown[] = []
  worker.use(
    handleOperatorWorkspacesGet({ body: { ...HOOLI, suspension: null } }),
    handleOperatorWorkspacesSuspend(async ({ request, params }) => {
      suspended.push({ id: params.workspaceId, body: await request.json() })
      return HttpResponse.json({ changed: true, workspace: HOOLI })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await screen.getByRole('button', { name: 'Suspend sending' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Suspend sending' }).click()
  await expect.element(dialog.getByText('A reason is required')).toBeInTheDocument()
  expect(suspended).toEqual([])

  await dialog.getByRole('textbox', { name: /Reason/ }).fill('  abuse report ')
  await dialog.getByRole('button', { name: 'Suspend sending' }).click()

  await expect.poll(() => suspended).toEqual([{ id: '5', body: { reason: 'abuse report' } }])
})

test('lifting a suspension asks for confirmation first', async () => {
  const lifted: string[] = []
  worker.use(
    handleOperatorWorkspacesGet({ body: HOOLI }),
    handleOperatorWorkspacesUnsuspend(({ params }) => {
      lifted.push(params.workspaceId)
      return HttpResponse.json({ changed: true, workspace: { ...HOOLI, suspension: null } })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await screen.getByRole('button', { name: 'Lift suspension' }).click()
  await expect
    .element(screen.getByRole('dialog').getByText('Lift the suspension?'))
    .toBeInTheDocument()
  expect(lifted).toEqual([])

  await screen.getByRole('dialog').getByRole('button', { name: 'Lift suspension' }).click()

  await expect.poll(() => lifted).toEqual(['5'])
})

test('a no-op says nothing changed', async () => {
  worker.use(
    handleOperatorWorkspacesGet({ body: HOOLI }),
    handleOperatorWorkspacesUnsuspend({ body: { changed: false, workspace: HOOLI } }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await screen.getByRole('button', { name: 'Lift suspension' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Lift suspension' }).click()

  await expect
    .element(screen.getByText('Nothing changed: sending was not suspended.'))
    .toBeInTheDocument()
})
