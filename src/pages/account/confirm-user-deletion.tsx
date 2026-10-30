import { Divider, Stack, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { Route as deleteRoute } from '../../routes/delete-account.tsx'
import { DeleteUserForm } from './DeleteUserForm.tsx'

// ConfirmUserDeletionPage is where the mailed link of a User with no password lands (User
// deletion, ADR 0050): the token of the link, the User's own email typed in and, with a Second
// factor, a code finish the deletion.
export function ConfirmUserDeletionPage() {
  const { t } = useTranslation()
  const { token } = deleteRoute.useSearch()

  return (
    <CenteredLayout maw={420}>
      <Stack>
        <Title order={2}>{t(($) => $.profile.deleteUserSectionTitle)}</Title>
        <Divider />
        <DeleteUserForm token={token} />
      </Stack>
    </CenteredLayout>
  )
}
