import * as z from 'zod'

import { jsonText } from './jsonText.ts'

// The editor's JSON document, as the API carries it: an opaque object the server checks.
export type MessageDoc = Record<string, unknown>

// A form holds a document as its JSON text (see jsonText); this is the text <-> object codec.
export const docCodec: z.ZodType<MessageDoc, string> = jsonText.pipe(z.record(z.string(), z.json()))
