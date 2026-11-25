import { useTranslation } from 'react-i18next'

import { LeaveDialog } from '../../components/LeaveDialog.tsx'

// Asks before the person leaves the editor with changes that leaving would lose; the editor
// session decides it, so what a pending Save carries does not ask.
export function LeaveGuard({ wouldLose }: { wouldLose: boolean }) {
  const { t } = useTranslation()

  return <LeaveDialog dirty={wouldLose} message={t(($) => $.automations.leaveMessage)} />
}
