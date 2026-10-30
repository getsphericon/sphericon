import { z } from 'zod'

// A same-origin path to go on to once signed in (a route's `redirect` search parameter); anything
// else, a protocol-relative `//host` included, is dropped.
export const zReturnPath = z
  .string()
  .startsWith('/')
  .refine((path) => !path.startsWith('//'))
  .optional()
  .catch(undefined)
