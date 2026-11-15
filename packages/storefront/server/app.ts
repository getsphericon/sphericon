import { zValidator } from '@hono/zod-validator'
import { contactsBatchUpsert, eventsCreate } from '@sphericon/node'
import { Hono } from 'hono'

import { COURSES, totalPrice } from '../src/courses.ts'
import { ENROLL_PATH, enrollSchema } from './enroll.ts'

// "Enroll" is a fake checkout: there is no payment provider, so the server only tells the
// platform what happened, through the generated SDK. A request the schema refuses is answered 400.
export function createApp() {
  return new Hono().post(ENROLL_PATH, zValidator('json', enrollSchema), async (c) => {
    const { email, subjectId, courses } = c.req.valid('json')

    const bought = COURSES.filter((course) => courses.includes(course.slug))
    if (bought.length !== courses.length) {
      return c.json({ error: 'Unknown course' }, 400)
    }

    await contactsBatchUpsert({
      body: { contacts: [{ email, subjectId }] },
      throwOnError: true,
    })
    await eventsCreate({
      body: {
        events: [
          {
            action: 'purchase',
            email,
            subjectId,
            properties: {
              amount: totalPrice(bought),
              currency: 'USD',
              courses,
            },
          },
        ],
      },
      throwOnError: true,
    })

    return c.json({ enrolled: courses })
  })
}

// The storefront's routes as the browser's typed client sees them.
export type StorefrontApp = ReturnType<typeof createApp>
