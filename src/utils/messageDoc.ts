import * as z from 'zod'

import type { EditorDocument } from '../generated/site/types.gen.ts'
import { jsonText } from './jsonText.ts'

// The editor's JSON document, as the API carries it: an opaque object the server checks.
export type MessageDoc = EditorDocument

// The visual editor's JSON export meets its document value here.
export const docCodec: z.ZodType<MessageDoc, string> = jsonText.pipe(z.record(z.string(), z.json()))
