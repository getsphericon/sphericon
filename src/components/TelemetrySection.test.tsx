import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteInstanceGetState,
  handleSiteInstanceSetTelemetry,
} from '../generated/site/msw.gen.ts'
import type { SiteInstanceState } from '../generated/site/types.gen.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { TelemetrySection } from './TelemetrySection.tsx'

const state = (telemetry: SiteInstanceState['telemetry']): SiteInstanceState => ({
  telemetry,
  noticeAcknowledged: true,
  telemetrySchemaUrl: 'https://docs.example.test/telemetry-schema',
})

test('an owner switches the report off', async () => {
  const bodies: string[] = []
  worker.use(
    handleSiteInstanceGetState({ body: state({ enabled: false, locked: false }) }),
    handleSiteInstanceSetTelemetry(async ({ request }) => {
      bodies.push(await request.text())
      return HttpResponse.json(state({ enabled: false, locked: false }))
    }),
  )
  const { screen } = await renderWithRouter(
    <TelemetrySection state={state({ enabled: true, locked: false })} />,
  )

  await screen.getByRole('switch', { name: 'Send the anonymous usage report' }).click()

  await expect.poll(() => bodies).toEqual(['{"enabled":false}'])
})

test('a setting decided by SPHERICON_TELEMETRY is locked and says why', async () => {
  const { screen } = await renderWithRouter(
    <TelemetrySection state={state({ enabled: true, locked: true, lockedBy: 'environment' })} />,
  )

  const toggle = screen.getByRole('switch', { name: 'Send the anonymous usage report' })
  await expect.element(toggle).toBeDisabled()
  await expect.element(toggle).toBeChecked()
  await expect
    .element(screen.getByText(/SPHERICON_TELEMETRY environment variable/))
    .toBeInTheDocument()
})

test('DO_NOT_TRACK locks the setting off and says why', async () => {
  const { screen } = await renderWithRouter(
    <TelemetrySection state={state({ enabled: false, locked: true, lockedBy: 'do_not_track' })} />,
  )

  const toggle = screen.getByRole('switch', { name: 'Send the anonymous usage report' })
  await expect.element(toggle).toBeDisabled()
  await expect.element(toggle).not.toBeChecked()
  await expect.element(screen.getByText(/DO_NOT_TRACK environment variable/)).toBeInTheDocument()
})
