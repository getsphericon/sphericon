import * as z from 'zod'

import { jsonText } from '../../utils/jsonText.ts'

// The contact form holds its tags as JSON text; a blank text is no tags.
export const tagsCodec: z.ZodType<string[], string> = jsonText.pipe(z.array(z.string()))
