import { definitionOf } from './definition.ts'
import type { Flow } from './definition.ts'

// What Save would send, as text: the editor is unsaved while this differs from the saved one.
export function automationSnapshot(name: string, flow: Flow): string {
  return JSON.stringify([name.trim(), definitionOf(flow)])
}
