import { configure } from '@sphericon/node'
import { afterEach, beforeEach, expect, test, vi } from 'vitest'

import { createApp } from './app.ts'

interface Received {
  path: string
  authorization: string | null
  body: unknown
}

// The outside boundary is the external API: the stub answers like it and keeps what it was sent.
let received: Received[]

beforeEach(() => {
  received = []
  configure({ baseUrl: 'https://api.test', token: 'demo-token' })
  vi.stubGlobal('fetch', async (request: Request) => {
    received.push({
      path: new URL(request.url).pathname,
      authorization: request.headers.get('authorization'),
      body: await request.json(),
    })

    return Response.json({})
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
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
  expect(received).toEqual([
    {
      path: '/contacts/batch',
      authorization: 'Bearer demo-token',
      body: {
        contacts: [
          { email: 'maya.learner@example.com', subjectId: 'user:maya.learner@example.com' },
        ],
      },
    },
    {
      path: '/events',
      authorization: 'Bearer demo-token',
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
    },
  ])
})
