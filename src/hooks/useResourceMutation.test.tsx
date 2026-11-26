import { useQuery } from '@tanstack/react-query'
import type { QueryKey } from '@tanstack/react-query'
import { expect, test, vi } from 'vitest'

import {
  siteSegmentsListQueryKey,
  siteSegmentsUpdateMutationKey,
  siteTagsListQueryKey,
  siteTemplatesListQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { useResourceMutation } from './useResourceMutation.ts'

const SAVE = 'Save'

// The generated keys carry their operation's tags in their first element.
const path = { workspaceId: 'w1' }
const segmentsKey = siteSegmentsListQueryKey({ path })
const templatesKey = siteTemplatesListQueryKey({ path })
const tagsKey = siteTagsListQueryKey({ path })

interface Fetches {
  segments: () => Promise<string>
  templates: () => Promise<string>
  tags: () => Promise<string>
}

function Harness({
  fetches,
  invalidate,
  onDone,
  refused = false,
  rereadOnError,
}: {
  fetches: Fetches
  invalidate?: QueryKey[] | false
  onDone?: () => void
  refused?: boolean
  rereadOnError?: boolean
}) {
  useQuery({ queryKey: segmentsKey, queryFn: fetches.segments })
  useQuery({ queryKey: templatesKey, queryFn: fetches.templates })
  useQuery({ queryKey: tagsKey, queryFn: fetches.tags })
  const save = useResourceMutation({
    mutation: {
      mutationKey: siteSegmentsUpdateMutationKey(),
      mutationFn: async () =>
        refused ? Promise.reject(new Error('refused')) : Promise.resolve('saved'),
    },
    ...(invalidate === undefined ? {} : { invalidate }),
    ...(rereadOnError === undefined ? {} : { rereadOnError }),
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
  await expect.poll(() => fetches.tags).toHaveBeenCalledTimes(1)

  await screen.getByRole('button', { name: SAVE }).click()

  await expect.poll(() => fetches.segments).toHaveBeenCalledTimes(2)
  expect(fetches.templates).toHaveBeenCalledTimes(1)
  expect(fetches.tags).toHaveBeenCalledTimes(1)
})

test('a listed query of another resource is refreshed too', async () => {
  const fetches = fetchSpies()
  const screen = await renderWithProviders(
    <Harness fetches={fetches} invalidate={[templatesKey]} />,
  )
  await expect.poll(() => fetches.tags).toHaveBeenCalledTimes(1)

  await screen.getByRole('button', { name: SAVE }).click()

  await expect.poll(() => fetches.templates).toHaveBeenCalledTimes(2)
  expect(fetches.segments).toHaveBeenCalledTimes(2)
  expect(fetches.tags).toHaveBeenCalledTimes(1)
})

test('invalidate false refreshes nothing', async () => {
  const fetches = fetchSpies()
  const onDone = vi.fn<() => void>()
  const screen = await renderWithProviders(
    <Harness fetches={fetches} invalidate={false} onDone={onDone} />,
  )
  await expect.poll(() => fetches.tags).toHaveBeenCalledTimes(1)

  await screen.getByRole('button', { name: SAVE }).click()

  await expect.poll(() => onDone).toHaveBeenCalledTimes(1)
  expect(fetches.segments).toHaveBeenCalledTimes(1)
})

test('a refused mutation refreshes nothing', async () => {
  const fetches = fetchSpies()
  const screen = await renderWithProviders(<Harness fetches={fetches} refused />)
  await expect.poll(() => fetches.tags).toHaveBeenCalledTimes(1)

  await screen.getByRole('button', { name: SAVE }).click()

  await expect.element(screen.getByText('Error')).toBeVisible()
  expect(fetches.segments).toHaveBeenCalledTimes(1)
})

test('rereadOnError refreshes on a refusal what a success would, and still says why', async () => {
  const fetches = fetchSpies()
  const screen = await renderWithProviders(
    <Harness fetches={fetches} invalidate={[templatesKey]} refused rereadOnError />,
  )
  await expect.poll(() => fetches.tags).toHaveBeenCalledTimes(1)

  await screen.getByRole('button', { name: SAVE }).click()

  await expect.poll(() => fetches.segments).toHaveBeenCalledTimes(2)
  await expect.poll(() => fetches.templates).toHaveBeenCalledTimes(2)
  expect(fetches.tags).toHaveBeenCalledTimes(1)
  await expect.element(screen.getByText('Error')).toBeVisible()
})
