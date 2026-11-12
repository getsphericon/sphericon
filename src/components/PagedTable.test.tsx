import { Button } from '@mantine/core'
import type { ComponentProps } from 'react'
import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import type { FieldColumn } from '../utils/tableColumns.ts'
import { PagedTable } from './PagedTable.tsx'

interface Row {
  id: string
}

interface Source {
  calls: () => number
  fail: (value: boolean) => void
  options: (paging: { page: number; pageSize: number }) => {
    queryKey: string[]
    queryFn: () => Promise<{ items: Row[]; totalItems: number }>
  }
}

// A list that answers with a row naming the call, and fails on demand.
function source(rows = 1): Source {
  let calls = 0
  let failing = false
  return {
    calls: () => calls,
    fail: (value) => {
      failing = value
    },
    options: () => ({
      queryKey: ['paged-table-test'],
      queryFn: async () => {
        calls += 1
        // Settled a tick later, as a request is.
        await Promise.resolve()
        if (failing) {
          throw new Error('boom')
        }
        return { items: rows ? [{ id: `row-${calls}` }] : [], totalItems: rows }
      },
    }),
  }
}

async function renderTable(
  list: Source,
  empty: ComponentProps<typeof PagedTable>['empty'] = { title: 'No rows' },
) {
  return await renderWithProviders(
    <PagedTable
      page={1}
      onPageChange={() => undefined}
      options={list.options}
      columns={[{ accessor: 'id', title: 'Id' } satisfies FieldColumn<Row>]}
      loadErrorTitle="Failed to load"
      collection="Rows"
      empty={empty}
    />,
  )
}

test('the refresh button fetches the current page again and the rows stay on screen', async () => {
  const list = source()
  const screen = await renderTable(list)
  await expect.element(screen.getByText('row-1')).toBeInTheDocument()
  const before = screen.getByRole('time').element().getAttribute('datetime')

  await screen.getByRole('button', { name: 'Refresh' }).click()

  await expect.element(screen.getByText('row-2')).toBeInTheDocument()
  expect(list.calls()).toBe(2)
  // The time moves on after a success, where a failure leaves it (the test below).
  expect(screen.getByRole('time').element().getAttribute('datetime')).not.toBe(before)
})

test('the time of the last refresh sits in a polite live region', async () => {
  const list = source()
  const screen = await renderTable(list)

  const region = screen.getByRole('status')
  await expect.element(region).toHaveAttribute('aria-live', 'polite')
  await expect.element(region.getByText(/Updated/)).toBeInTheDocument()
  await expect.element(region.getByRole('time')).toBeInTheDocument()
})

test('a failed refresh leaves the previous time in place', async () => {
  const list = source()
  const screen = await renderTable(list)
  await expect.element(screen.getByText('row-1')).toBeInTheDocument()
  const before = screen.getByRole('time').element().getAttribute('datetime')

  list.fail(true)
  await screen.getByRole('button', { name: 'Refresh' }).click()
  await expect.element(screen.getByText('Failed to load').first()).toBeInTheDocument()

  expect(screen.getByRole('time').element().getAttribute('datetime')).toBe(before)
})

test('an empty list shows its heading, the description and the one action under the count', async () => {
  let clicked = 0
  const label = ['Add', 'row'].join(' ')
  const screen = await renderTable(source(0), {
    title: 'No rows yet',
    description: 'Rows appear once something happens.',
    action: (
      <Button
        onClick={() => {
          clicked += 1
        }}
      >
        {label}
      </Button>
    ),
  })

  await expect.element(screen.getByText('Rows (0)')).toBeInTheDocument()
  await expect.element(screen.getByText('No rows yet')).toBeInTheDocument()
  await expect.element(screen.getByText('Rows appear once something happens.')).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Add row' }).click()
  expect(clicked).toBe(1)
})

test('the count follows the total of the collection', async () => {
  const screen = await renderTable(source())

  await expect.element(screen.getByText('Rows (1)')).toBeInTheDocument()
})

test('a full-page list is its page header: the title, the actions, the toolbar and its count', async () => {
  const createLabel = ['Create', 'row'].join(' ')
  const toolbarLabel = ['Tool', 'bar'].join('')
  const list = source(3)
  const screen = await renderWithProviders(
    <PagedTable
      page={1}
      onPageChange={() => undefined}
      options={list.options}
      columns={[{ accessor: 'id', title: 'Id' } satisfies FieldColumn<Row>]}
      loadErrorTitle="Failed to load"
      collection="Rows"
      empty={{ title: 'No rows' }}
      pageHeader={{
        actions: <Button>{createLabel}</Button>,
        toolbar: <span>{toolbarLabel}</span>,
        count: (total) => `${total} rows`,
      }}
    />,
  )

  await expect.element(screen.getByRole('heading', { name: 'Rows' })).toBeInTheDocument()
  await expect.element(screen.getByText('3 rows')).toBeInTheDocument()
  await expect.element(screen.getByRole('button', { name: 'Create row' })).toBeInTheDocument()
  await expect.element(screen.getByText('Toolbar')).toBeInTheDocument()
  // The table does not repeat the title above itself.
  await expect.element(screen.getByText('Rows (3)')).not.toBeInTheDocument()
})

// Whether an empty state is on screen: mantine-datatable keeps its own in the DOM and marks it
// data-active only while it shows.
function emptyStateShown() {
  return [...document.body.querySelectorAll('*')].some(
    (element) =>
      element.childElementCount === 0 &&
      element.textContent === 'No rows' &&
      element.closest('.mantine-datatable-empty-state:not([data-active])') === null,
  )
}

test('a list with rows never shows its empty state while the first page loads', async () => {
  let flashed = false
  const observer = new MutationObserver(() => {
    flashed ||= emptyStateShown()
  })
  observer.observe(document.body, { childList: true, subtree: true, attributes: true })

  const screen = await renderTable(source())
  await expect.element(screen.getByText('row-1')).toBeInTheDocument()
  observer.disconnect()

  expect(flashed).toBe(false)
})
