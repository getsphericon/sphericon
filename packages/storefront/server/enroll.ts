import { z } from 'zod'

// The contract between the browser and the storefront's own server: the browser posts here, the
// server answers by calling the external API.
export const ENROLL_PATH = '/enroll'

export const enrollSchema = z.object({
  email: z.email(),
  // The same subject id the browser passed to `identify`, so both sides land on one Contact.
  subjectId: z.string().min(1),
  courses: z.array(z.string()).min(1),
})

export type EnrollRequest = z.infer<typeof enrollSchema>
