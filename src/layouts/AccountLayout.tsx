import { AccountNavbar } from '../components/AccountNavbar.tsx'
import { InstanceNotice } from '../components/InstanceNotice.tsx'
import { UpdateBanner } from '../components/UpdateBanner.tsx'
import { UserMenu } from '../components/UserMenu.tsx'
import { DashboardShell } from './DashboardShell.tsx'

// AccountLayout is the shell for workspace-independent account pages (profile,
// and future account-level settings): account sidebar, no workspace switcher.
export function AccountLayout() {
  return (
    <DashboardShell
      sidebar={<AccountNavbar />}
      headerRight={<UserMenu />}
      banner={
        <>
          <UpdateBanner />
          <InstanceNotice />
        </>
      }
    />
  )
}
