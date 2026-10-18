import { expect, test } from 'vitest'

import { handleSiteInstancePreviewTelemetryReport } from '../generated/site/msw.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { TelemetryReportActions } from './TelemetryReportActions.tsx'

test('the Owner reads the report as it would be sent and finds the schema', async () => {
  worker.use(
    handleSiteInstancePreviewTelemetryReport({
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
  worker.use(handleSiteInstancePreviewTelemetryReport(() => problem(500)))
  const { screen } = await renderWithRouter(
    <TelemetryReportActions schemaUrl="https://docs.example.test/telemetry-schema" />,
  )

  await screen.getByRole('button', { name: 'Preview the report' }).click()

  await expect.element(screen.getByText('Could not build the report.')).toBeInTheDocument()
})
