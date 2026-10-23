import { client } from './generated/external/client.gen.ts'

export * from './generated/external/index.ts'

export interface SphericonConfig {
  /** Origin of the external API, e.g. `https://api.sphericon.localhost` (no trailing path). */
  baseUrl: string
  /** An API token of the Workspace, sent as `Authorization: Bearer <token>`. */
  token: string
}

/** Points every SDK call (`eventsCreate`, `contactsCreate`, ...) at one API and token. */
export function configure({ baseUrl, token }: SphericonConfig): void {
  client.setConfig({ baseUrl, auth: token })
}
