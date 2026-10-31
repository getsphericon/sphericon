import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleStaffInstanceAcknowledgeNotice,
  handleStaffInstanceGetState,
  handleStaffInstanceSetTelemetry,
} from '../generated/staff/msw.gen.ts'
import { StaffInstanceTelemetryLock } from '../generated/staff/types.gen.ts'
import type { StaffInstanceState } from '../generated/staff/types.gen.ts'
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
