import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleStaffWorkspaceAuditList,
  handleStaffWorkspacesClearSendCap,
  handleStaffWorkspacesExempt,
  handleStaffWorkspacesGet,
  handleStaffWorkspacesSetSendCap,
  handleStaffWorkspacesSuspend,
  handleStaffWorkspacesUnexempt,
  handleStaffWorkspacesUnsuspend,
} from '../generated/staffee/msw.gen.ts'
import type { StaffWorkspaceDetailResource } from '../generated/staffee/types.gen.ts'
import { Route as consoleWorkspaceRoute } from '../routes/console/_authed/_enterprise/workspaces.$workspaceId.tsx'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { ConsoleWorkspacePage } from './workspace.tsx'

// Mounted at the route's id, not its fullPath: the console's guard route is pathless, so the
// two differ and `consoleWorkspaceRoute.useParams()` looks the match up by id.
const MOUNT = {
  path: consoleWorkspaceRoute.id,
  initialPath: consoleWorkspaceRoute.id.replace('$workspaceId', 'HooliWs5'),
}

const HOOLI: StaffWorkspaceDetailResource = {
  id: 'HooliWs5',
  name: 'Hooli',
  createdAt: '2026-01-01T00:00:00Z',
  suspension: {
    at: '2026-02-01T00:00:00Z',
    actor: { kind: 'staff', id: 'op-fixture' },
    reason: 'abuse report',
  },
  deliverability: { windowHours: 24, volumeFloor: 1000, sendVolume: 0, domains: [] },
  holds: [],
  exempt: false,
  warned: false,
  abuse: { exempt: false },
  sendCaps: false,
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
  worker.use(handleStaffWorkspaceAuditList({ body: { items: [] } }))
})

test('lists the audit log with a staff member shown as Sphericon staff, and loads more', async () => {
  worker.use(
    handleStaffWorkspacesGet({ body: HOOLI }),
    handleStaffWorkspaceAuditList(({ request }) =>
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
                actor: { kind: 'staff', name: 'Sphericon staff' },
                action: 'workspace.suspend',
                target: { type: 'workspace', id: '5', name: 'Hooli' },
              },
            ],
            nextCursor: '2',
          }),
    ),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('Sphericon staff')).toBeInTheDocument()
  await expect.element(screen.getByText('workspace.suspend')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Load more' }).click()
  await expect.element(screen.getByText('tag.create')).toBeInTheDocument()
  await expect.element(screen.getByText('Peter')).toBeInTheDocument()
})

test('a hold change in the audit log names the credential', async () => {
  worker.use(
    handleStaffWorkspacesGet({ body: HOOLI }),
    handleStaffWorkspaceAuditList({
      body: {
        items: [
          {
            id: '3',
            occurredAt: '2026-02-01T00:00:00Z',
            actor: { kind: 'service', id: '4', name: 'hold-service' },
            action: 'hold.create',
            target: { type: 'hold', name: 'review' },
          },
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('hold-service')).toBeInTheDocument()
})

test('the audit log names a system actor and an API token by its name or its id', async () => {
  worker.use(
    handleStaffWorkspacesGet({ body: HOOLI }),
    handleStaffWorkspaceAuditList({
      body: {
        items: [
          {
            id: '5',
            occurredAt: '2026-02-01T00:00:00Z',
            actor: { kind: 'system' },
            action: 'workspace.suspend',
            target: { type: 'workspace', id: '1' },
          },
          {
            id: '4',
            occurredAt: '2026-02-01T00:00:00Z',
            actor: { kind: 'api_token', id: '7' },
            action: 'tag.create',
            target: { type: 'tag', name: 'vip' },
          },
          {
            id: '3',
            occurredAt: '2026-02-01T00:00:00Z',
            actor: { kind: 'api_token', id: '8', name: 'Zapier' },
            action: 'tag.delete',
            target: { type: 'tag' },
          },
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('System', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('API token #7')).toBeInTheDocument()
  await expect.element(screen.getByText('Zapier')).toBeInTheDocument()
  await expect.element(screen.getByText('workspace #1')).toBeInTheDocument()
  await expect.element(screen.getByText('tag vip')).toBeInTheDocument()
})

test('an empty audit log says so', async () => {
  worker.use(handleStaffWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('No recorded changes')).toBeInTheDocument()
})

test('shows the metadata and who suspended the workspace, and why', async () => {
  worker.use(handleStaffWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByRole('heading', { name: 'Hooli' })).toBeInTheDocument()
  await expect.element(screen.getByText('HooliWs5', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Staff member op-fixture')).toBeInTheDocument()
  await expect.element(screen.getByText('abuse report')).toBeInTheDocument()
})

test('a workspace that is not suspended says so', async () => {
  worker.use(handleStaffWorkspacesGet({ body: { ...HOOLI, suspension: null } }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('Sending is not suspended.')).toBeInTheDocument()
})

test('a non-staff actor shows its kind, not an id', async () => {
  worker.use(
    handleStaffWorkspacesGet({
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
  worker.use(handleStaffWorkspacesGet(() => problem(404, { detail: 'workspace not found' })))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('workspace not found')).toBeInTheDocument()
})

test('shows the send volume and the rates with their counts per sending domain', async () => {
  worker.use(
    handleStaffWorkspacesGet({
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
    handleStaffWorkspacesGet({
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
  worker.use(handleStaffWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('No sending domains.')).toBeInTheDocument()
})

test('suspending needs a reason, and sends it only after the confirm button', async () => {
  const suspended: unknown[] = []
  worker.use(
    handleStaffWorkspacesGet({ body: { ...HOOLI, suspension: null } }),
    handleStaffWorkspacesSuspend(async ({ request, params }) => {
      suspended.push({ id: params.workspaceId, body: await request.json() })
      return HttpResponse.json({ changed: true, workspace: HOOLI })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await screen.getByRole('button', { name: 'Suspend sending' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Suspend sending' }).click()
  await expect.element(dialog.getByText('This field is required')).toBeInTheDocument()
  expect(suspended).toEqual([])

  await dialog.getByRole('textbox', { name: /Reason/ }).fill('  abuse report ')
  await dialog.getByRole('button', { name: 'Suspend sending' }).click()

  await expect.poll(() => suspended).toEqual([{ id: 'HooliWs5', body: { reason: 'abuse report' } }])
})

test('lifting a suspension asks for confirmation first', async () => {
  const lifted: string[] = []
  worker.use(
    handleStaffWorkspacesGet({ body: HOOLI }),
    handleStaffWorkspacesUnsuspend(({ params }) => {
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

  await expect.poll(() => lifted).toEqual(['HooliWs5'])
})

test('a no-op says nothing changed', async () => {
  worker.use(
    handleStaffWorkspacesGet({ body: HOOLI }),
    handleStaffWorkspacesUnsuspend({ body: { changed: false, workspace: HOOLI } }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await screen.getByRole('button', { name: 'Lift suspension' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Lift suspension' }).click()

  await expect
    .element(screen.getByText('Nothing changed: sending was not suspended.'))
    .toBeInTheDocument()
})

test('lists the active holds with key, reason code, message, since, expiry and setter', async () => {
  worker.use(
    handleStaffWorkspacesGet({
      body: {
        ...HOOLI,
        holds: [
          {
            key: 'k1',
            reasonCode: 'review',
            message: 'Sending is paused for now.',
            createdAt: '2026-03-01T00:00:00Z',
            expiresAt: '2026-03-08T00:00:00Z',
            setBy: 'service-a',
          },
          {
            key: 'k2',
            reasonCode: 'quota_reached',
            message: 'Quota reached.',
            createdAt: '2026-03-01T00:00:00Z',
            expiresAt: '2026-03-08T00:00:00Z',
            setBy: 'service-b',
          },
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByRole('heading', { name: 'Staff holds' })).toBeInTheDocument()
  await expect.element(screen.getByText('k1', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('review', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Sending is paused for now.')).toBeInTheDocument()
  await expect.element(screen.getByText('service-a', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('quota_reached', { exact: true })).toBeInTheDocument()
})

test('a workspace without holds says so', async () => {
  worker.use(handleStaffWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('No active holds.')).toBeInTheDocument()
})

test('shows the warning the detector computes now and when the owners were last told', async () => {
  worker.use(
    handleStaffWorkspacesGet({
      body: {
        ...HOOLI,
        suspension: null,
        warned: true,
        abuse: {
          exempt: false,
          warning: { domain: 'mail.hooli.test', metric: 'complaint', rate: 0.0015 },
          clearedAt: '2026-02-02T00:00:00Z',
          warningNotifiedAt: '2026-02-03T00:00:00Z',
        },
      },
    }),
  )

  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect
    .element(screen.getByText('Complaint rate 0.15% on mail.hooli.test'))
    .toBeInTheDocument()
  await expect.element(screen.getByText('Owners told of freeze')).toBeInTheDocument()
  await expect.element(screen.getByText('Never', { exact: true })).toBeInTheDocument()
})

test('exempting asks for confirmation first, then calls the API', async () => {
  const exempted: string[] = []
  worker.use(
    handleStaffWorkspacesGet({ body: HOOLI }),
    handleStaffWorkspacesExempt(({ params }) => {
      exempted.push(params.workspaceId)
      return HttpResponse.json({ changed: true, workspace: { ...HOOLI, exempt: true } })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await screen.getByRole('button', { name: 'Exempt from detector' }).click()
  await expect
    .element(screen.getByRole('dialog').getByText('Exempt this Workspace?'))
    .toBeInTheDocument()
  expect(exempted).toEqual([])

  await screen.getByRole('dialog').getByRole('button', { name: 'Exempt from detector' }).click()

  await expect.poll(() => exempted).toEqual(['HooliWs5'])
})

test('an exempt workspace offers to police it again', async () => {
  const policed: string[] = []
  worker.use(
    handleStaffWorkspacesGet({
      body: { ...HOOLI, exempt: true, abuse: { exempt: true } },
    }),
    handleStaffWorkspacesUnexempt(({ params }) => {
      policed.push(params.workspaceId)
      return HttpResponse.json({ changed: true, workspace: HOOLI })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('Exempt', { exact: true })).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Police again' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Police again' }).click()

  await expect.poll(() => policed).toEqual(['HooliWs5'])
})

test('a self-hosted instance shows no Send cap', async () => {
  worker.use(handleStaffWorkspacesGet({ body: HOOLI }))
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('Abuse detector')).toBeInTheDocument()
  await expect.element(screen.getByText('Send cap', { exact: true })).not.toBeInTheDocument()
})

test('setting a Send cap checks the number, then sends it', async () => {
  const set: unknown[] = []
  worker.use(
    handleStaffWorkspacesGet({ body: { ...HOOLI, sendCaps: true } }),
    handleStaffWorkspacesSetSendCap(async ({ request, params }) => {
      set.push({ id: params.workspaceId, body: await request.json() })
      return HttpResponse.json({ changed: true, workspace: { ...HOOLI, sendCap: 5000 } })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText(/^No Send cap/)).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Set Send cap' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect.element(dialog.getByText('This field is required')).toBeInTheDocument()
  expect(set).toEqual([])

  await dialog.getByRole('textbox', { name: /Messages per 24 hours/ }).fill('5000')
  await dialog.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => set).toEqual([{ id: 'HooliWs5', body: { sendCap: 5000 } }])
})

test('a Send cap shows, and removing it asks first', async () => {
  const cleared: string[] = []
  worker.use(
    handleStaffWorkspacesGet({ body: { ...HOOLI, sendCaps: true, sendCap: 1000 } }),
    handleStaffWorkspacesClearSendCap(({ params }) => {
      cleared.push(params.workspaceId)
      return HttpResponse.json({ changed: true, workspace: HOOLI })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleWorkspacePage />, MOUNT)

  await expect.element(screen.getByText('1,000 messages per 24 hours')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Remove Send cap' }).click()
  expect(cleared).toEqual([])
  await screen.getByRole('dialog').getByRole('button', { name: 'Remove Send cap' }).click()

  await expect.poll(() => cleared).toEqual(['HooliWs5'])
})
