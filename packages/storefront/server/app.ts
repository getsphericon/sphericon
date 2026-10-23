import { contactsBatchUpsert, eventsCreate } from '@sphericon/node'
import { Hono } from 'hono'

import { COURSES } from '../src/courses.ts'
import { ENROLL_PATH, enrollSchema } from './enroll.ts'

// "Enroll" is a fake checkout: there is no payment provider, so the server only tells the
// platform what happened, through the generated SDK.
export function createApp(): Hono {
  const app = new Hono()

  app.post(ENROLL_PATH, async (c) => {
    const parsed = enrollSchema.safeParse(await c.req.json())
    if (!parsed.success) {
      return c.json({ error: parsed.error.message }, 400)
    }
    const { email, subjectId, courses } = parsed.data

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
              amount: bought.reduce((sum, course) => sum + course.price, 0),
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

  return app
}
