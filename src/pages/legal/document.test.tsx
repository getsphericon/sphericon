import { HttpResponse } from 'msw'
import { expect, test } from 'vitest'

import { handleSiteLegalDocumentsGet } from '../../generated/site/msw.gen.ts'
import { Route as legalDocumentRoute } from '../../routes/legal.$key.tsx'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { routeMount } from '../../test/routeMount.ts'
import { worker } from '../../test/worker.ts'
import { LegalDocumentPage } from './document.tsx'

const mount = routeMount(legalDocumentRoute, { key: 'terms' })

test('shows the current version of the document, its Markdown rendered', async () => {
  const keys: string[] = []
  worker.use(
    handleSiteLegalDocumentsGet(({ params }) => {
      keys.push(params.key)
      return HttpResponse.json({
        key: 'terms',
        title: 'Terms of service',
        version: '2026-10',
        markdown: '## Using the service\n\nBe **kind**.',
        publishedAt: '2026-10-01T00:00:00Z',
      })
    }),
  )

  const { screen } = await renderWithRouter(<LegalDocumentPage />, mount)

  await expect
    .element(screen.getByRole('heading', { name: 'Terms of service' }))
    .toBeInTheDocument()
  await expect
    .element(screen.getByRole('heading', { name: 'Using the service' }))
    .toBeInTheDocument()
  await expect.element(screen.getByText('kind', { exact: true })).toBeInTheDocument()
  await expect.element(screen.getByText('Version 2026-10', { exact: false })).toBeInTheDocument()
  expect(keys).toEqual(['terms'])
})

test('an unknown document says it is not found', async () => {
  worker.use(handleSiteLegalDocumentsGet(() => problem(404, { detail: 'no such legal document' })))

  const { screen } = await renderWithRouter(<LegalDocumentPage />, mount)

  await expect.element(screen.getByText('This document does not exist.')).toBeInTheDocument()
})
