import { contactsBatchUpsert, eventsCreate } from '@sphericon/node'
import { beforeEach, expect, test, vi } from 'vitest'

import { createApp } from './app.ts'

vi.mock('@sphericon/node', () => ({
  contactsBatchUpsert: vi.fn<() => Promise<{ data: object }>>(() => Promise.resolve({ data: {} })),
  eventsCreate: vi.fn<() => Promise<{ data: undefined }>>(() =>
    Promise.resolve({ data: undefined }),
  ),
}))

beforeEach(() => {
  vi.mocked(contactsBatchUpsert).mockClear()
  vi.mocked(eventsCreate).mockClear()
})

test('Enroll upserts the Contact and records purchase with the email and subject id of the request', async () => {
  const response = await createApp().request('/enroll', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      email: 'maya.learner@example.com',
      subjectId: 'user:maya.learner@example.com',
      courses: ['git-in-a-day', 'sql-for-analysts'],
    }),
  })

  expect(response.status).toBe(200)
  expect(contactsBatchUpsert).toHaveBeenCalledWith({
    body: {
      contacts: [{ email: 'maya.learner@example.com', subjectId: 'user:maya.learner@example.com' }],
    },
    throwOnError: true,
  })
  expect(eventsCreate).toHaveBeenCalledWith({
    body: {
      events: [
        {
          action: 'purchase',
          email: 'maya.learner@example.com',
          subjectId: 'user:maya.learner@example.com',
          properties: {
            amount: 78,
            currency: 'USD',
            courses: ['git-in-a-day', 'sql-for-analysts'],
          },
        },
      ],
    },
    throwOnError: true,
  })
})
