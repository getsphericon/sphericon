import { HttpResponse } from 'msw'
import { afterEach, expect, test, vi } from 'vitest'

import {
  handleSiteTokensCreate,
  handleSiteTokensDelete,
  handleSiteTokensList,
} from '../../generated/site/msw.gen.ts'
import type { SiteApiTokenResource } from '../../generated/site/types.gen.ts'
import { problem } from '../../test/problem.ts'
import { renderWithRouter } from '../../test/renderWithRouter.tsx'
import { worker } from '../../test/worker.ts'
import { ApiKeysSection } from './ApiKeysSection.tsx'

afterEach(() => {
  vi.restoreAllMocks()
})

test('creates a token and reveals the one-time secret', async () => {
  let created = false
  worker.use(
    handleSiteTokensCreate(() => {
      created = true
      return HttpResponse.json(
        {
          token: 'omtk_newprefix_supersecret',
          resource: {
            id: '2',
            name: 'CI',
            prefix: 'newprefix',
            scopes: ['contacts:read'],
            createdAt: '2026-06-01T00:00:00Z',
          },
        },
        { status: 201 },
      )
    }),
    // GET list: empty before creation, the created token after.
    handleSiteTokensList(() =>
      HttpResponse.json(
        created
          ? [
              {
                id: '2',
                name: 'CI',
                prefix: 'newprefix',
                scopes: ['contacts:read'],
                createdAt: '2026-06-01T00:00:00Z',
              },
            ]
          : [],
      ),
    ),
  )

  const { screen } = await renderWithRouter(<ApiKeysSection slug="test" />)

  await screen.getByLabelText(/^Token name/).fill('CI')
  await screen.getByRole('button', { name: 'Create token' }).click()

  // The full secret is shown once.
  await expect.element(screen.getByText('omtk_newprefix_supersecret')).toBeInTheDocument()
})

test('a plain member who is forbidden to create a token sees why', async () => {
  worker.use(
    handleSiteTokensCreate(() =>
      problem(403, { detail: 'only owners and admins can manage API tokens' }),
    ),
    handleSiteTokensList({ body: [] }),
  )

  const { screen } = await renderWithRouter(<ApiKeysSection slug="test" />)

  await screen.getByLabelText(/^Token name/).fill('CI')
  await screen.getByRole('button', { name: 'Create token' }).click()

  await expect
    .element(screen.getByText(/Only workspace owners and admins can manage API tokens/))
    .toBeInTheDocument()
})

const REVOKE_FAILED = 'Failed to revoke token'

const token: SiteApiTokenResource = {
  id: '2',
  name: 'CI',
  prefix: 'newprefix',
  scopes: ['contacts:read'],
  createdAt: '2026-06-01T00:00:00Z',
}

const listTokens = (items: SiteApiTokenResource[]) => handleSiteTokensList({ body: items })

test('revokes a token after confirmation', async () => {
  let revoked = false
  worker.use(
    listTokens([token]),
    handleSiteTokensDelete(() => {
      revoked = true
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const { screen } = await renderWithRouter(<ApiKeysSection slug="test" />)

  await screen.getByRole('button', { name: 'Revoke' }).click()
  await expect.element(screen.getByText('Revoke token?')).toBeInTheDocument()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.poll(() => revoked).toBe(true)
})

test('a failed revoke shows the error toast', async () => {
  worker.use(
    listTokens([token]),
    handleSiteTokensDelete(() => problem(500, { detail: 'boom' })),
  )
  const { screen } = await renderWithRouter(<ApiKeysSection slug="test" />)

  await screen.getByRole('button', { name: 'Revoke' }).click()
  await screen.getByRole('dialog').getByRole('button', { name: 'Delete' }).click()

  await expect.element(screen.getByText(REVOKE_FAILED)).toBeInTheDocument()
})

test('shows an error alert when the tokens fail to load', async () => {
  worker.use(handleSiteTokensList(() => problem(500, { detail: 'boom' })))
  const { screen } = await renderWithRouter(<ApiKeysSection slug="test" />)

  await expect
    .element(screen.getByRole('alert', { name: 'Failed to load tokens' }))
    .toBeInTheDocument()
})

test('the one-time secret can be dismissed', async () => {
  worker.use(
    listTokens([]),
    handleSiteTokensCreate({
      body: { token: 'omtk_secret_value', resource: token },
      status: 201,
    }),
  )
  const { screen } = await renderWithRouter(<ApiKeysSection slug="test" />)

  await screen.getByLabelText(/^Token name/).fill('CI')
  await screen.getByRole('button', { name: 'Create token' }).click()
  await expect.element(screen.getByText('omtk_secret_value')).toBeInTheDocument()

  await screen
    .getByRole('alert', { name: /Copy this token now/ })
    .getByRole('button')
    .last()
    .click()
  await expect.element(screen.getByText('omtk_secret_value')).not.toBeInTheDocument()
})
