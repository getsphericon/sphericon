import * as z from 'zod'

import { jsonText } from './jsonText.ts'

// The editor's JSON document, as the API carries it: an opaque object the server checks.
export type MessageDoc = Record<string, unknown>

// The visual editor's JSON export meets its document value here.
export const docCodec: z.ZodType<MessageDoc, string> = jsonText.pipe(z.record(z.string(), z.json()))
