import { expect, test } from 'vitest'

import { handleStaffInstancePreviewTelemetryReport } from '../generated/staff/msw.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { TelemetryReportActions } from './TelemetryReportActions.tsx'

test('the Staff member reads the report as it would be sent and finds the schema', async () => {
  worker.use(
    handleStaffInstancePreviewTelemetryReport({
      body: { schemaVersion: 1, workspaces: '1-9', webhooksConfigured: false },
    }),
  )
  const { screen } = await renderWithRouter(
    <TelemetryReportActions schemaUrl="https://docs.example.test/telemetry-schema" />,
  )

  await expect
    .element(screen.getByRole('link', { name: 'What the report contains' }))
    .toHaveAttribute('href', 'https://docs.example.test/telemetry-schema')
  await screen.getByRole('button', { name: 'Preview the report' }).click()

  await expect.element(screen.getByText(/"workspaces": "1-9"/)).toBeInTheDocument()
})

test('a report that cannot be built says so', async () => {
  worker.use(handleStaffInstancePreviewTelemetryReport(() => problem(500)))
  const { screen } = await renderWithRouter(
    <TelemetryReportActions schemaUrl="https://docs.example.test/telemetry-schema" />,
  )

  await screen.getByRole('button', { name: 'Preview the report' }).click()

  await expect.element(screen.getByText('Could not build the report.')).toBeInTheDocument()
})
