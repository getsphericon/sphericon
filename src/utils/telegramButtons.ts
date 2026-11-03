import * as z from 'zod'

import type { TelegramButton } from '../generated/site/types.gen.ts'

const buttons = z.array(z.object({ label: z.string(), url: z.string() }))

// The buttons of a Telegram node are held on the canvas as JSON text: its form holds only strings.
export const buttonsCodec = {
  read(text: unknown): TelegramButton[] {
    const parsed = buttons.safeParse(typeof text === 'string' && text !== '' ? safeJson(text) : [])
    return parsed.success ? parsed.data : []
  },
  write(list: TelegramButton[]): string {
    return list.length === 0 ? '' : JSON.stringify(list)
  },
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text)
  } catch {
    return []
  }
}
