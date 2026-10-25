import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import {
  handleSiteInstanceAcknowledgeNotice,
  handleSiteInstanceGetState,
} from '../generated/site/msw.gen.ts'
import type { SiteInstanceState } from '../generated/site/types.gen.ts'
import { problem } from '../test/problem.ts'
import { renderWithRouter } from '../test/renderWithRouter.tsx'
import { worker } from '../test/worker.ts'
import { InstanceNotice } from './InstanceNotice.tsx'

const state = (over: Partial<SiteInstanceState> = {}): SiteInstanceState => ({
  telemetry: { enabled: true, locked: false },
  noticeAcknowledged: false,
  telemetrySchemaUrl: 'https://docs.example.test/telemetry-schema',
  ...over,
})

test('an owner sees the notice until they acknowledge it', async () => {
  let current = state()
  worker.use(
    handleSiteInstanceGetState(() => HttpResponse.json(current)),
    handleSiteInstanceAcknowledgeNotice(() => {
      current = state({ noticeAcknowledged: true })
      return HttpResponse.json(current)
    }),
  )
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect.element(screen.getByText('Anonymous usage report')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Got it' }).click()

  await expect.element(screen.getByText('Anonymous usage report')).not.toBeInTheDocument()
})

test('the notice says the environment decides when the setting is locked', async () => {
  worker.use(
    handleSiteInstanceGetState({
      body: state({ telemetry: { enabled: true, locked: true, lockedBy: 'environment' } }),
    }),
  )
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect
    .element(screen.getByText(/The server environment decides whether it is sent/))
    .toBeInTheDocument()
})

test('the notice is not shown while the environment keeps the report off', async () => {
  worker.use(
    handleSiteInstanceGetState({
      body: state({ telemetry: { enabled: false, locked: true, lockedBy: 'environment' } }),
    }),
  )
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect.poll(() => screen.getByText('Anonymous usage report').query()).toBeNull()
})

test('an acknowledged notice is not shown again', async () => {
  worker.use(handleSiteInstanceGetState({ body: state({ noticeAcknowledged: true }) }))
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect.poll(() => screen.getByText('Anonymous usage report').query()).toBeNull()
})

test('someone who is not an owner of the oldest workspace sees nothing', async () => {
  worker.use(handleSiteInstanceGetState(() => problem(403)))
  const { screen } = await renderWithRouter(<InstanceNotice />)

  await expect.poll(() => screen.getByText('Anonymous usage report').query()).toBeNull()
})
