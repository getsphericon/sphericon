import { getRequestListener } from '@hono/node-server'
import { configure } from '@sphericon/node'
import type { Plugin } from 'vite'

import { createApp } from './app.ts'
import { ENROLL_PATH } from './enroll.ts'
import { requiredEnv } from './env.ts'

// Runs the storefront's Hono server inside the Vite dev server, so the one `demo` daemon serves
// both and the browser reaches it on its own origin. The daemon exports DEMO_API_URL (the
// external API at https://api.<app host>) and DEMO_API_TOKEN (the "Demo site" token), and sets
// NODE_USE_SYSTEM_CA so Node trusts the certificate `caddy trust` installed.
export function storefrontServer(): Plugin {
  return {
    name: 'storefront:server',
    configureServer(server) {
      configure({ baseUrl: requiredEnv('DEMO_API_URL'), token: requiredEnv('DEMO_API_TOKEN') })

      const listener = getRequestListener(createApp().fetch)
      server.middlewares.use((req, res, next) => {
        if (req.url === ENROLL_PATH) {
          void listener(req, res)
        } else {
          next()
        }
      })
    },
  }
}
