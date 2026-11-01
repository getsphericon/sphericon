import { HttpResponse } from 'msw'
import { beforeEach, expect, test, vi } from 'vitest'
import { userEvent } from 'vitest/browser'

import {
  handleSiteAuditExport,
  handleSiteAuditFacets,
  handleSiteAuditGetRetention,
  handleSiteAuditList,
  handleSiteAuditSetRetention,
} from '../../generated/site/msw.gen.ts'
import type { SiteAuditEntryResource } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import type { AuditFilter } from '../../utils/auditFilter.ts'
import { AuditExportButton, AuditLogSection, ChangeHistoryLink } from './AuditLogSection.tsx'

const WORKSPACE_ID = 'TestWs01'

function entry(over: Partial<SiteAuditEntryResource> = {}): SiteAuditEntryResource {
  return {
    id: '2',
    occurredAt: '2026-01-02T03:04:05Z',
    actor: { kind: 'user', id: '1', name: 'John' },
    action: 'membership.update',
    target: { type: 'membership', id: '3', name: 'Mary' },
    diff: { role: { from: 'member', to: 'admin' } },
    ...over,
  }
}

// With audit:configure the section reads the retention window; tests that care override it.
beforeEach(() => {
  worker.use(
    handleSiteAuditGetRetention({ body: { retentionDays: 90 } }),
    handleSiteAuditFacets({
      body: {
        actors: [
          { kind: 'api_token', id: '77' },
          { kind: 'user', id: '2', name: 'Jane' },
        ],
        actions: ['integration.update', 'membership.update'],
        targetTypes: ['integration', 'membership'],
      },
    }),
  )
})

test('lists who changed what, with the change', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByRole('combobox', { name: 'Action' })).toBeInTheDocument()
  await expect.element(screen.getByText('John', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('cell', { name: "Changed a member's role" }))
    .toBeInTheDocument()
  await expect.element(screen.getByText('membership Mary')).toBeInTheDocument()
})

test('expands an entry to its before/after diff', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('admin', { exact: true })).not.toBeInTheDocument()
  await screen.getByRole('button', { name: 'Show details' }).click()
  await expect.element(screen.getByText('role', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('member', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('admin', { exact: true })).toBeInTheDocument()
})

test('shows a sensitive field as changed only', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry({ diff: { secret: 'changed' } })] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'Show details' }).click()
  await expect.element(screen.getByText('secret', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('changed', { exact: true })).toBeInTheDocument()
})

test('a blank side of a change reads empty and a structured one is shown as JSON', async () => {
  worker.use(
    handleSiteAuditList({
      body: { items: [entry({ diff: { params: { from: null, to: { limit: 5 } } } })] },
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'Show details' }).click()
  await expect.element(screen.getByText('empty', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('{"limit":5}', { exact: true })).toBeInTheDocument()
})

test('an actor known by neither name nor id is named by its kind', async () => {
  worker.use(
    handleSiteAuditList({
      body: {
        items: [
          entry({ id: '6', actor: { kind: 'api_token' } }),
          entry({ id: '7', actor: { kind: 'user' } }),
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('API token #', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText("Changed a member's role").nth(1)).toBeInTheDocument()
})

test('an applied actor the log no longer holds stays selectable', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ actorKind: 'user', actorId: '99' }}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByRole('combobox', { name: 'Actor' })).toHaveValue('99')
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
  const filter = { targetType: 'integration', targetId: '5', ip: '10.0.0.1' }
  const { screen } = await renderWithRouter(
    <>
      <AuditExportButton workspaceId={WORKSPACE_ID} filter={filter} />
      <AuditLogSection
        workspaceId={WORKSPACE_ID}
        filter={filter}
        onFilterChange={vi.fn<() => void>()}
        canConfigureRetention
      />
    </>,
  )

  await expect
    .poll(() =>
      queries.some((q) => q.includes('targetType=integration') && q.includes('targetId=5')),
    )
    .toBe(true)
  expect(queries.some((q) => q.includes('ip=10.0.0.1'))).toBe(true)
  await expect
    .element(screen.getByRole('combobox', { name: 'Target type' }))
    .toHaveValue('integration')

  await screen.getByRole('button', { name: 'Export CSV' }).click()
  await expect.poll(() => exported).toContain('targetType=integration')
  expect(exported).toContain('targetId=5')
})

test('choosing an actor or an action among those of the log applies it at once', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const onFilterChange = vi.fn<(filter: AuditFilter) => void>()
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ ip: '10.0.0.1' }}
      onFilterChange={onFilterChange}
      canConfigureRetention
    />,
  )

  await screen.getByRole('combobox', { name: 'Actor' }).click()
  await screen.getByRole('option', { name: 'Jane' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({
    ip: '10.0.0.1',
    actorKind: 'user',
    actorId: '2',
  })

  await screen.getByRole('combobox', { name: 'Action' }).click()
  await screen.getByRole('option', { name: /Changed an integration/ }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({ ip: '10.0.0.1', action: 'integration.update' })
})

test('a filter the log no longer holds stays selected', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ actorKind: 'user', actorId: '9', action: 'contact.delete' }}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByRole('combobox', { name: 'Actor' })).toHaveValue('9')
  await expect
    .element(screen.getByRole('combobox', { name: 'Action' }))
    .toHaveValue('Deleted a contact')
  await screen.getByRole('combobox', { name: 'Actor' }).click()
  await expect.element(screen.getByRole('option', { name: 'API token #77' })).toBeInTheDocument()
})

test('the system as the actor filter is named, though it has no id', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ actorKind: 'system' }}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByRole('combobox', { name: 'Actor' })).toHaveValue('System')
})

test('a period preset narrows the log to its days', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const onFilterChange = vi.fn<(filter: AuditFilter) => void>()
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ ip: '10.0.0.1' }}
      onFilterChange={onFilterChange}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'Period' }).click()
  await screen.getByRole('button', { name: 'Last 7 days' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({
    ip: '10.0.0.1',
    from: expect.any(String),
    to: expect.any(String),
  })
})

test('a chip removes its filter, and reset clears every filter', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const onFilterChange = vi.fn<(filter: AuditFilter) => void>()
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ targetType: 'integration', targetId: '5' }}
      onFilterChange={onFilterChange}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'Remove the filter Target #5' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({ targetType: 'integration' })

  await screen.getByRole('button', { name: 'Reset filters' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({})
})

test('a value of an entry narrows the log to the entries sharing it', async () => {
  worker.use(
    handleSiteAuditList({ body: { items: [entry({ requestId: 'req-1', ip: '10.0.0.1' })] } }),
  )
  const onFilterChange = vi.fn<(filter: AuditFilter) => void>()
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ action: 'membership.update' }}
      onFilterChange={onFilterChange}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'membership Mary' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({
    action: 'membership.update',
    targetType: 'membership',
    targetId: '3',
  })

  await screen.getByRole('button', { name: 'Show details' }).click()
  await screen.getByRole('button', { name: 'req-1' }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({
    action: 'membership.update',
    requestId: 'req-1',
  })
})

test('says so when the filter matches nothing', async () => {
  worker.use(handleSiteAuditList({ body: { items: [] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      filter={{ ip: '1.2.3.4' }}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
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
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('No audited changes yet')).toBeInTheDocument()
})

test('offers the next page while a cursor is returned', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()], nextCursor: '2' } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument()
})

test('loading more appends the next page below the first', async () => {
  const cursors: (string | null)[] = []
  worker.use(
    handleSiteAuditList(({ request }) => {
      const cursor = new URL(request.url).searchParams.get('cursor')
      cursors.push(cursor)
      return HttpResponse.json(
        cursor
          ? { items: [entry({ id: '3', action: 'membership.delete' })] }
          : { items: [entry()], nextCursor: '2' },
      )
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'Load more' }).click()

  await expect.element(screen.getByText('Removed a member')).toBeInTheDocument()
  await expect
    .element(screen.getByRole('cell', { name: "Changed a member's role" }))
    .toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Load more' })).not.toBeInTheDocument()
  expect(cursors).toContain('2')
})

test.each([402, 403])('renders nothing when the API answers %i', async (status) => {
  worker.use(handleSiteAuditList(() => problem(status, { detail: 'refused' })))
  const { screen } = await renderWithRouter(
    <>
      <AuditExportButton workspaceId={WORKSPACE_ID} filter={{}} />
      <AuditLogSection
        workspaceId={WORKSPACE_ID}
        onFilterChange={vi.fn<() => void>()}
        canConfigureRetention
      />
    </>,
  )

  await expect.element(screen.getByRole('combobox', { name: 'Action' })).not.toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Export CSV' })).not.toBeInTheDocument()
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
    <AuditExportButton workspaceId={WORKSPACE_ID} filter={{}} />,
  )

  await screen.getByRole('button', { name: 'Export CSV' }).click()
  await expect.poll(() => exported).toBe(1)
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
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect
    .element(screen.getByText('Entries older than 90 days are removed automatically.'))
    .toBeInTheDocument()
  await screen.getByRole('button', { name: 'Change retention' }).click()
  const dialog = screen.getByRole('dialog')
  const input = dialog.getByLabelText('Keep the log for (days)')
  await expect.element(input).toHaveValue('90')
  await input.fill('30')
  await userEvent.keyboard('{Enter}')
  await expect.poll(() => saved).toEqual({ retentionDays: 30 })
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
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await screen.getByRole('button', { name: 'Change retention' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText('Keep the log for (days)').fill('')
  await userEvent.keyboard('{Enter}')
  await expect.poll(() => saved).toEqual({ retentionDays: null })
})

test('a failed retention save is reported', async () => {
  worker.use(
    handleSiteAuditList({ body: { items: [entry()] } }),
    handleSiteAuditGetRetention({ body: { retentionDays: null } }),
    handleSiteAuditSetRetention(() => problem(422, { detail: 'bad window' })),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('Entries are kept forever.')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Change retention' }).click()
  const dialog = screen.getByRole('dialog')
  await dialog.getByLabelText('Keep the log for (days)').fill('5')
  await userEvent.keyboard('{Enter}')
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
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByRole('combobox', { name: 'Action' })).toBeInTheDocument()
  await expect.poll(() => retentionAnswered).toBeGreaterThan(0)
  await expect
    .element(screen.getByRole('button', { name: 'Change retention' }))
    .not.toBeInTheDocument()
})

test('a change shows an empty side as empty and a number as written', async () => {
  worker.use(
    handleSiteAuditList({ body: { items: [entry({ diff: { note: { from: '', to: 42 } } })] } }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention={false}
    />,
  )

  await screen.getByRole('button', { name: 'Show details' }).click()
  await expect.element(screen.getByText('empty', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('42', { exact: true })).toBeInTheDocument()
  await expect
    .element(screen.getByRole('button', { name: 'Change retention' }))
    .not.toBeInTheDocument()
})

test('a hold change is attributed to Sphericon staff', async () => {
  worker.use(
    handleSiteAuditList({
      body: {
        items: [
          entry({
            id: '8',
            actor: { kind: 'staff', name: 'Sphericon staff' },
            action: 'hold.create',
            target: { type: 'hold', name: 'review' },
            diff: null,
          }),
        ],
      },
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

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
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('API token #7')).toBeInTheDocument()
  await expect.element(screen.getByRole('cell', { name: 'System' })).toBeInTheDocument()
  await expect.element(screen.getByText('tag #9')).toBeInTheDocument()
})

test('names an API token actor by its name when it has one', async () => {
  worker.use(
    handleSiteAuditList({
      body: { items: [entry({ actor: { kind: 'api_token', id: '12', name: 'CI deploy' } })] },
    }),
  )
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('CI deploy', { exact: true })).toBeInTheDocument()
})

test('reports a failed load of the log', async () => {
  worker.use(handleSiteAuditList(() => problem(500)))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await expect.element(screen.getByText('Failed to load the audit log').first()).toBeInTheDocument()
})

test('an action reads as words, its code on hover', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={vi.fn<() => void>()}
      canConfigureRetention
    />,
  )

  await screen.getByRole('cell', { name: "Changed a member's role" }).getByText(/role/).hover()

  await expect.element(screen.getByRole('tooltip', { name: 'membership.update' })).toBeVisible()
})

test('the action filter finds an action by its words or by its code, and filters by the code', async () => {
  worker.use(handleSiteAuditList({ body: { items: [entry()] } }))
  const onFilterChange = vi.fn<(filter: AuditFilter) => void>()
  const { screen } = await renderWithRouter(
    <AuditLogSection
      workspaceId={WORKSPACE_ID}
      onFilterChange={onFilterChange}
      canConfigureRetention
    />,
  )
  const action = screen.getByRole('combobox', { name: 'Action' })

  await action.fill('role')
  await expect
    .element(screen.getByRole('option', { name: /Changed a member's role/ }))
    .toBeVisible()
  await expect
    .element(screen.getByRole('option', { name: /Changed an integration/ }))
    .not.toBeInTheDocument()

  await action.fill('integration.up')
  await expect
    .element(screen.getByRole('option', { name: /Changed a member's role/ }))
    .not.toBeInTheDocument()
  await screen.getByRole('option', { name: /Changed an integration/ }).click()
  expect(onFilterChange).toHaveBeenLastCalledWith({ action: 'integration.update' })
})
