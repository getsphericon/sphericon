import { HttpResponse } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'

import {
  handleSiteAuditExport,
  handleSiteAuditGetRetention,
  handleSiteAuditList,
  handleSiteAuditSetRetention,
} from '../../generated/site/msw.gen.ts'
import type { SiteAuditEntryResource } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import type { AuditFilter } from './auditFilter.ts'
import { AuditLogSection, ChangeHistoryLink } from './AuditLogSection.tsx'

const WORKSPACE_ID = 'TestWs01'

function entry(over: Partial<SiteAuditEntryResource> = {}): SiteAuditEntryResource {
  return {
    id: '2',
    occurredAt: '2026-01-02T03:04:05Z',
    actor: { kind: 'user', id: '1', name: 'John' },
    action: 'membership.update',
    billing: false,
    target: { type: 'membership', id: '3', name: 'Mary' },
    diff: { role: { from: 'member', to: 'admin' } },
    ...over,
  }
}

// The section always reads the retention window; tests that care override it.
beforeEach(() => {
  worker.use(handleSiteAuditGetRetention({ body: { retentionDays: 90 } }))
})

test('lists who changed what, with the change', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('Audit log')).toBeInTheDocument()
  await expect.element(screen.getByText('John', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('membership.update')).toBeInTheDocument()
  await expect.element(screen.getByText('membership Mary')).toBeInTheDocument()
})

test('expands an entry to its before/after diff', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('admin', { exact: true })).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Show changes' }).click()
  await expect.element(screen.getByText('role', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('member', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('admin', { exact: true })).toBeInTheDocument()
})

test('shows a sensitive field as changed only', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry({ diff: { secret: 'changed' } })] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await screen.getByRole('button', { name: 'Show changes' }).click()
  await expect.element(screen.getByText('secret', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('changed', { exact: true })).toBeInTheDocument()
})

test('sends the applied filter to the list and to the export', async () => {
  const queries: string[] = []
  let exported = ''
  worker.use(
    handleSiteAuditList(({ request }) => {
      queries.push(new URL(request.url).search)
      return HttpResponse.json({ items: [entry()] })
    }),
    handleSiteAuditExport(({ request }) => {
      exported = new URL(request.url).search
      return new HttpResponse('id\n', { headers: { 'content-type': 'text/csv' } })
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ targetType: 'integration', targetId: '5', ip: '10.0.0.1' }}
      onFilterChange={() => {}}
    />,
  )

  await vi.waitFor(() =>
    expect(
      queries.some((q) => q.includes('targetType=integration') && q.includes('targetId=5')),
    ).toBe(true),
  )
  expect(queries.some((q) => q.includes('ip=10.0.0.1'))).toBe(true)
  await expect.element(screen.getByLabelText('Target type')).toHaveValue('integration')

  await screen.getByRole('button', { name: 'Export CSV' }).click()
  await vi.waitFor(() => expect(exported).toContain('targetType=integration'))
  expect(exported).toContain('targetId=5')
})

test('applying the form hands the new filter up, and reset clears it', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const onFilterChange = vi.fn<(filter: AuditFilter) => void>()
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={onFilterChange} />,
  )

  await screen.getByLabelText('Request id').fill(' req-1 ')
  await screen.getByLabelText('Target type').fill('webhook_endpoint')
  await screen.getByRole('button', { name: 'Apply filters' }).click()
  expect(onFilterChange).toHaveBeenCalledWith({
    requestId: 'req-1',
    targetType: 'webhook_endpoint',
  })

  await screen.getByRole('button', { name: 'Reset filters' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({})
})

test('says so when the filter matches nothing', async () => {
  worker.use(handleSiteAuditList({ body: { items: [] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ ip: '1.2.3.4' }}
      onFilterChange={() => {}}
    />,
  )

  await expect.element(screen.getByText('No entries match the filters')).toBeInTheDocument()
})

test('the change history link opens the log filtered to the object', async () => {
  worker.use(handleSiteAuditList({ body: { items: [] } }))
  const { screen } = await renderWithRouter(
    <ChangeHistoryLink workspaceId={WORKSPACE_ID} targetType="integration" targetId="5" />,
  )

  const link = screen.getByRole('link', { name: 'Change history' })
  await expect.element(link).toBeInTheDocument()
  const href = link.element().getAttribute('href') ?? ''
  expect(href).toContain('targetType=integration')
  expect(href).toContain('targetId=%225%22')
})

test('the change history link is not offered without the log', async () => {
  worker.use(handleSiteAuditList(() => problem(402, { detail: 'no' })))
  const { screen } = await renderWithRouter(
    <ChangeHistoryLink workspaceId={WORKSPACE_ID} targetType="integration" targetId="5" />,
  )

  await expect.element(screen.getByRole('link', { name: 'Change history' })).not.toBeInTheDocument()
})

test('shows the empty state', async () => {
  worker.use(handleSiteAuditList({ body: { items: [] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('No audited changes yet')).toBeInTheDocument()
})

test('offers the next page while a cursor is returned', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()], nextCursor: '2' } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument()
})

test.each([402, 403])('renders nothing when the API answers %i', async (status) => {
  worker.use(handleSiteAuditList(() => problem(status, { detail: 'refused' })))
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('Audit log')).not.toBeInTheDocument()
})

test('exports the log as CSV through the generated client', async () => {
  let exported = 0
  worker.use(
    handleSiteAuditList({ body: { items: [entry()] } }),
    handleSiteAuditExport(() => {
      exported += 1
      return new HttpResponse('id,action\n2,membership.update\n', {
        headers: { 'content-type': 'text/csv' },
      })
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await screen.getByRole('button', { name: 'Export CSV' }).click()
  await vi.waitFor(() => expect(exported).toBe(1))
})

test('shows the retention window and saves a new one', async () => {
  let saved: unknown
  worker.use(
    handleSiteAuditList({ body: { items: [entry()] } }),
    handleSiteAuditGetRetention({ body: { retentionDays: 90 } }),
    handleSiteAuditSetRetention(async ({ request }) => {
      saved = await request.json()
      return HttpResponse.json({ retentionDays: null })
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  const input = screen.getByLabelText('Keep the log for (days)')
  await expect.element(input).toHaveValue('90')
  await input.fill('30')
  await screen.getByRole('button', { name: 'Save retention' }).click()
  await vi.waitFor(() => expect(saved).toEqual({ retentionDays: 30 }))
})

test('an empty retention window keeps the log forever', async () => {
  let saved: unknown
  worker.use(
    handleSiteAuditList({ body: { items: [entry()] } }),
    handleSiteAuditGetRetention({ body: { retentionDays: 90 } }),
    handleSiteAuditSetRetention(async ({ request }) => {
      saved = await request.json()
      return HttpResponse.json({ retentionDays: null })
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await screen.getByLabelText('Keep the log for (days)').fill('')
  await screen.getByRole('button', { name: 'Save retention' }).click()
  await vi.waitFor(() => expect(saved).toEqual({ retentionDays: null }))
})

test('a failed retention save is reported', async () => {
  worker.use(
    handleSiteAuditList({ body: { items: [entry()] } }),
    handleSiteAuditGetRetention({ body: { retentionDays: null } }),
    handleSiteAuditSetRetention(() => problem(422, { detail: 'bad window' })),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await screen.getByLabelText('Keep the log for (days)').fill('5')
  await screen.getByRole('button', { name: 'Save retention' }).click()
  await expect.element(screen.getByText('Could not save the retention')).toBeInTheDocument()
})

test('the retention control is not offered without its license', async () => {
  let retentionAnswered = 0
  worker.use(
    handleSiteAuditList({ body: { items: [entry()] } }),
    handleSiteAuditGetRetention(() => {
      retentionAnswered++
      return problem(402, { detail: 'no' })
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('Audit log')).toBeInTheDocument()
  await expect.poll(() => retentionAnswered).toBeGreaterThan(0)
  await expect.element(screen.getByLabelText('Keep the log for (days)')).not.toBeInTheDocument()
})

test('a billing hold change is shown as billing activity, attributed to Sphericon staff', async () => {
  worker.use(
    handleSiteAuditList({
      body: {
        items: [
          entry({
            id: '8',
            actor: { kind: 'staff', name: 'Sphericon staff' },
            action: 'hold.create',
            billing: true,
            target: { type: 'hold', name: 'billing' },
            diff: null,
          }),
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('Billing', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'Sphericon staff' })).toBeInTheDocument()
})

test('an entry without a diff or a name falls back gracefully', async () => {
  worker.use(
    handleSiteAuditList({
      body: {
        items: [
          entry({
            id: '3',
            actor: { kind: 'api_token', id: '7' },
            target: { type: 'tag', id: '9' },
            diff: null,
          }),
          entry({ id: '4', actor: { kind: 'system' }, target: { type: 'webhook_endpoint' } }),
          entry({
            id: '5',
            actor: { kind: 'user', id: '2' },
            diff: { a: { from: null, to: '' }, b: { from: { x: 1 }, to: 2 } },
          }),
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection workspaceId={WORKSPACE_ID} onFilterChange={() => {}} />,
  )

  await expect.element(screen.getByText('API token #7')).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'System' })).toBeInTheDocument()
  await expect.element(screen.getByText('tag #9')).toBeInTheDocument()
})
