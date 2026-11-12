import { Outlet, useMatch } from '@tanstack/react-router'

import { Route as consoleRoute } from '../routes/console/route.tsx'
import { RegionalNumbers } from './RegionalNumbers.tsx'

// AppRoot is the root route's component: what every page shares and no layout owns. The Staff
// console keeps the Instance's language, since it has no User preference (ADR 0096).
export function AppRoot() {
  const inConsole = useMatch({ from: consoleRoute.id, shouldThrow: false }) !== undefined
  return (
    <>
      <RegionalNumbers instance={inConsole} />
      <Outlet />
    </>
  )
}
