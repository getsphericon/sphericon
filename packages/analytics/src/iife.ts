import type { TrackerConfig } from './index.ts'
import { initTracking } from './index.ts'

// Config from the script tag itself, e.g.
//   <script async src="https://host/t.js" data-collect-key="..." data-collect-url="https://host"></script>
// When loaded via the async stub instead, document.currentScript is the injected tag and
// configuration arrives through the _omq `init` command, resolved inside initTracking().
function configFromScript(): TrackerConfig | undefined {
  const script = document.currentScript instanceof HTMLScriptElement ? document.currentScript : null
  const collectKey = script?.dataset.collectKey

  if (!collectKey) {
    return undefined
  }

  return { collectKey, baseUrl: script.dataset.collectUrl ?? '' }
}

initTracking(configFromScript())
