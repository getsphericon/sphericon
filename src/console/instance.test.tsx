import { HttpResponse } from 'msw'
import { beforeEach, expect, test } from 'vitest'

import {
  handleStaffInstanceAcknowledgeNotice,
  handleStaffInstanceClearDefaultSendCap,
  handleStaffInstanceGetState,
  handleStaffInstanceListAuditEntries,
  handleStaffInstanceSetDefaultSendCap,
  handleStaffInstanceSetTelemetry,
} from '../generated/staff/msw.gen.ts'
import { StaffInstanceTelemetryLock } from '../generated/staff/types.gen.ts'
import type { StaffDefaultSendCapInput, StaffInstanceState } from '../generated/staff/types.gen.ts'
import { Route as consoleInstanceRoute } from '../routes/console/_authed/instance.tsx'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { routeMount } from '../test/routeMount.ts'
import { worker } from '../test/worker.ts'
import { ConsoleInstancePage } from './instance.tsx'
import { InstanceNotice } from './InstanceNotice.tsx'

const PAGE = routeMount(consoleInstanceRoute)
const SWITCH = 'Send the anonymous usage report'

const state = (over: Partial<StaffInstanceState> = {}): StaffInstanceState => ({
  telemetry: { enabled: true, locked: false },
  noticeAcknowledged: true,
  telemetrySchemaUrl: 'https://docs.example.test/telemetry-schema',
  ...over,
})

// The page reads the Instance audit log; a test about it installs its own.
beforeEach(() => {
  worker.use(handleStaffInstanceListAuditEntries({ body: { items: [] } }))
})

test('a Staff member switches the report off', async () => {
  let current = state()
  worker.use(
    handleStaffInstanceGetState(() => HttpResponse.json(current)),
    handleStaffInstanceSetTelemetry(async ({ request }) => {
      const { enabled } = await request.json()
      current = state({ telemetry: { enabled, locked: false } })
      return HttpResponse.json(current)
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)
  const toggle = screen.getByRole('switch', { name: SWITCH })
  await expect.element(toggle).toBeChecked()

  await toggle.click()

  await expect.element(toggle).not.toBeChecked()
  await expect.element(screen.getByText('Usage report setting saved')).toBeInTheDocument()
})

test('DO_NOT_TRACK locks the setting off and says why', async () => {
  worker.use(
    handleStaffInstanceGetState({
      body: state({
        telemetry: {
          enabled: false,
          locked: true,
          lockedBy: StaffInstanceTelemetryLock.DO_NOT_TRACK,
        },
      }),
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  const toggle = screen.getByRole('switch', { name: SWITCH })
  await expect.element(toggle).toBeDisabled()
  await expect.element(toggle).not.toBeChecked()
  await expect.element(screen.getByText(/DO_NOT_TRACK environment variable/)).toBeInTheDocument()
})

test('the page tells of a newer release and of an expired license key', async () => {
  worker.use(
    handleStaffInstanceGetState({
      body: state({ latestRelease: '0.4.0', licenseExpiredAt: '2026-06-01T00:00:00Z' }),
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  await expect.element(screen.getByText(/Version 0\.4\.0 of Sphericon/)).toBeInTheDocument()
  await expect.element(screen.getByText('The license key has expired')).toBeInTheDocument()
})

test('the notice shows until a Staff member acknowledges it', async () => {
  let current = state({ noticeAcknowledged: false })
  worker.use(
    handleStaffInstanceGetState(() => HttpResponse.json(current)),
    handleStaffInstanceAcknowledgeNotice(() => {
      current = state({ noticeAcknowledged: true })
      return HttpResponse.json(current)
    }),
  )
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect.element(screen.getByText('Anonymous usage report')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Got it' }).click()

  await expect.element(screen.getByText('Anonymous usage report')).not.toBeInTheDocument()
})

test('the notice is not shown while the environment keeps the report off', async () => {
  worker.use(
    handleStaffInstanceGetState({
      body: state({
        noticeAcknowledged: false,
        telemetry: {
          enabled: false,
          locked: true,
          lockedBy: StaffInstanceTelemetryLock.ENVIRONMENT,
        },
      }),
    }),
  )
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect.poll(() => screen.getByText('Anonymous usage report').query()).toBeNull()
})

test('an instance without Send caps shows no default Send cap and no warning', async () => {
  worker.use(handleStaffInstanceGetState({ body: state() }))
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  await expect.element(screen.getByRole('switch', { name: SWITCH })).toBeInTheDocument()
  expect(screen.getByText('Default Send cap').query()).toBeNull()
  expect(screen.getByText('No default Send cap').query()).toBeNull()
})

test('warns while no default Send cap is set, and a Staff member sets one', async () => {
  let current = state({ defaultSendCap: { perDay: null } })
  const set: StaffDefaultSendCapInput[] = []
  worker.use(
    handleStaffInstanceGetState(() => HttpResponse.json(current)),
    handleStaffInstanceSetDefaultSendCap(async ({ request }) => {
      const body = await request.json()
      set.push(body)
      current = state({ defaultSendCap: { perDay: body.perDay } })
      return HttpResponse.json({ changed: true, defaultSendCap: { perDay: body.perDay } })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  await expect.element(screen.getByText('No default Send cap')).toBeInTheDocument()
  await expect
    .element(screen.getByText(/New workspaces have no Send cap\. Set a default/))
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Set default' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('textbox', { name: /Messages per 24 hours/ }).fill('5000')
  await dialog.getByRole('button', { name: 'Save' }).click()

  await expect.poll(() => set).toEqual([{ perDay: 5000 }])
  await expect.element(screen.getByText('Default Send cap saved')).toBeInTheDocument()
  await expect.element(screen.getByText('5,000 messages per 24 hours')).toBeInTheDocument()
  await expect.element(screen.getByText('No default Send cap')).not.toBeInTheDocument()
})

test('a default Send cap outside the bounds is refused before anything is sent', async () => {
  const set: unknown[] = []
  worker.use(
    handleStaffInstanceGetState({ body: state({ defaultSendCap: { perDay: null } }) }),
    handleStaffInstanceSetDefaultSendCap(async ({ request }) => {
      set.push(await request.json())
      return HttpResponse.json({ changed: true, defaultSendCap: { perDay: 1 } })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  await screen.getByRole('button', { name: 'Set default' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByRole('textbox', { name: /Messages per 24 hours/ }).fill('0')
  await dialog.getByRole('button', { name: 'Save' }).click()
  await expect
    .element(dialog.getByText('Enter a whole number from 1 to 100,000,000.'))
    .toBeInTheDocument()
  expect(set).toEqual([])
})

test('shows the default Send cap and removes it only after the confirm button', async () => {
  let cleared = 0
  worker.use(
    handleStaffInstanceGetState({ body: state({ defaultSendCap: { perDay: 2000 } }) }),
    handleStaffInstanceClearDefaultSendCap(() => {
      cleared++
      return HttpResponse.json({ changed: true, defaultSendCap: { perDay: null } })
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  await expect.element(screen.getByText('2,000 messages per 24 hours')).toBeInTheDocument()
  expect(screen.getByText('No default Send cap').query()).toBeNull()
  await screen.getByRole('button', { name: 'Remove default' }).click()
  await expect
    .element(screen.getByRole('dialog').getByText('Remove the default Send cap?'))
    .toBeInTheDocument()
  expect(cleared).toBe(0)

  await screen.getByRole('dialog').getByRole('button', { name: 'Remove default' }).click()

  await expect.poll(() => cleared).toBe(1)
  await expect.element(screen.getByText('Default Send cap removed')).toBeInTheDocument()
})

test('the Instance audit log says who changed which setting from what to what', async () => {
  worker.use(
    handleStaffInstanceGetState({ body: state({ defaultSendCap: { perDay: 2000 } }) }),
    handleStaffInstanceListAuditEntries({
      body: {
        items: [
          {
            id: '2',
            occurredAt: '2026-10-31T12:00:00Z',
            actor: { id: '7', email: 'ops@example.com' },
            action: 'instance.default_send_cap_change',
            diff: { default_send_cap: { from: 500, to: 2000 } },
          },
          {
            id: '1',
            occurredAt: '2026-10-30T12:00:00Z',
            actor: { id: '7', email: 'ops@example.com' },
            action: 'instance.default_send_cap_change',
            diff: { default_send_cap: { to: 500 } },
          },
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(<ConsoleInstancePage />, PAGE)

  await expect.element(screen.getByText('Changed the default Send cap').first()).toBeInTheDocument()
  await expect.element(screen.getByText('default_send_cap: 500 → 2000')).toBeInTheDocument()
  await expect.element(screen.getByText('default_send_cap: none → 500')).toBeInTheDocument()
  await expect.element(screen.getByText('ops@example.com').first()).toBeInTheDocument()
})
