import { useQuery } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'
import { expect, test, vi } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useResourceMutation } from './useResourceMutation.ts'

const SAVE = 'Save'

// The generated keys carry their operation's tags in their first element.
const segmentsKey = [{ _id: 'siteSegmentsList', tags: ['Site Segments'] }]
const templatesKey = [{ _id: 'siteTemplatesList', tags: ['Site Templates'] }]
const tagsKey = [{ _id: 'siteTagsList', tags: ['Site Tags'] }]

interface Fetches {
  segments: () => Promise<string>
  templates: () => Promise<string>
  tags: () => Promise<string>
}

function Harness({
  fetches,
  invalidate,
  onDone,
}: {
  fetches: Fetches
  invalidate?: QueryKey[] | false
  onDone?: () => void
}) {
  useQuery({ queryKey: segmentsKey, queryFn: fetches.segments })
  useQuery({ queryKey: templatesKey, queryFn: fetches.templates })
  useQuery({ queryKey: tagsKey, queryFn: fetches.tags })
  const save = useResourceMutation({
    mutation: {
      mutationKey: [{ _id: 'siteSegmentsUpdate', tags: ['Site Segments'] }],
      mutationFn: async () => Promise.resolve('saved'),
    },
    ...(invalidate === undefined ? {} : { invalidate }),
    errorTitle: 'Error',
    ...(onDone ? { onDone } : {}),
  })
  return (
    <button type="button" onClick={() => save.mutate()}>
      {SAVE}
    </button>
  )
}

function fetchSpies() {
  return {
    segments: vi.fn<() => Promise<string>>(async () => Promise.resolve('segments')),
    templates: vi.fn<() => Promise<string>>(async () => Promise.resolve('templates')),
    tags: vi.fn<() => Promise<string>>(async () => Promise.resolve('tags')),
  }
}

test('a mutation refreshes the queries of its own tag and no others', async () => {
  const fetches = fetchSpies()
  const screen = await renderWithProviders(<Harness fetches={fetches} />)
  await vi.waitFor(() => expect(fetches.tags).toHaveBeenCalledTimes(1))

  await screen.getByRole('button', { name: SAVE }).click()

  await vi.waitFor(() => expect(fetches.segments).toHaveBeenCalledTimes(2))
  expect(fetches.templates).toHaveBeenCalledTimes(1)
  expect(fetches.tags).toHaveBeenCalledTimes(1)
})

test('a listed query of another resource is refreshed too', async () => {
  const fetches = fetchSpies()
  const screen = await renderWithProviders(
    <Harness fetches={fetches} invalidate={[templatesKey]} />,
  )
  await vi.waitFor(() => expect(fetches.tags).toHaveBeenCalledTimes(1))

  await screen.getByRole('button', { name: SAVE }).click()

  await vi.waitFor(() => expect(fetches.templates).toHaveBeenCalledTimes(2))
  expect(fetches.segments).toHaveBeenCalledTimes(2)
  expect(fetches.tags).toHaveBeenCalledTimes(1)
})

test('invalidate false refreshes nothing', async () => {
  const fetches = fetchSpies()
  const onDone = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <Harness fetches={fetches} invalidate={false} onDone={onDone} />,
  )
  await vi.waitFor(() => expect(fetches.tags).toHaveBeenCalledTimes(1))

  await screen.getByRole('button', { name: SAVE }).click()

  await vi.waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
  expect(fetches.segments).toHaveBeenCalledTimes(1)
})
