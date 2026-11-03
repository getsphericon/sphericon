import { Outlet, useLocation } from '@tanstack/react-router'

import { RegionalNumbers } from './RegionalNumbers.tsx'

// AppRoot is the root route's component: what every page shares and no layout owns. The Staff
// console keeps the Instance's language, since it has no User preference (ADR 0096).
export function AppRoot() {
  const inConsole = useLocation({ select: ({ pathname }) => pathname.startsWith('/console') })
  return (
    <>
      <RegionalNumbers instance={inConsole} />
      <Outlet />
    </>
  )
}
