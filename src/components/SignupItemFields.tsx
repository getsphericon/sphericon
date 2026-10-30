import { Checkbox } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { SignupItemKind } from '../generated/site/types.gen.ts'
import type { SiteSignupItem } from '../generated/site/types.gen.ts'
import { Route as legalDocumentRoute } from '../routes/legal.$key.tsx'
import { getFieldError } from '../utils/apiErrors.ts'
import { signupItemField } from '../utils/signupItems.ts'
import { AnchorLink } from './RouterLink.tsx'

interface SignupItemFieldsProps {
  // The Sign-up items in effect, in the order of the form.
  items: readonly SiteSignupItem[]
  // The boxes the person ticked, by item key: none at first.
  ticked: Readonly<Record<string, boolean>>
  onTick: (key: string, checked: boolean) => void
  // The form's last refusal, whose `signupItems.<key>` messages go under their boxes.
  error: unknown
}

// The Sign-up items a form that sets up an account asks (ADR 0062): each required acceptance and
// optional consent as an unticked checkbox linking to its Legal document, each notice as a link.
export function SignupItemFields({ items, ticked, onTick, error }: SignupItemFieldsProps) {
  const { t } = useTranslation()
  return items.map((item) =>
    item.kind === SignupItemKind.NOTICE ? (
      <AnchorLink
        key={item.key}
        to={legalDocumentRoute.to}
        params={{ key: item.key }}
        target="_blank"
        size="sm"
      >
        {item.title}
      </AnchorLink>
    ) : (
      <Checkbox
        key={item.key}
        label={item.title}
        checked={ticked[item.key] === true}
        onChange={(event) => onTick(item.key, event.currentTarget.checked)}
        description={
          <AnchorLink
            to={legalDocumentRoute.to}
            params={{ key: item.key }}
            target="_blank"
            size="xs"
          >
            {t(($) => $.signUp.readItem, { title: item.title })}
          </AnchorLink>
        }
        error={getFieldError(error, signupItemField(item.key))}
      />
    ),
  )
}
