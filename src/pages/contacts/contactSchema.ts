import { zSiteUpdateContactInput } from '../../generated/site/zod.gen.ts'
import { resourceFormSchema } from '../../resources/resourceFormSchema.ts'

// Contact custom fields are not edited here, so they never round-trip into the payload. The
// subject id is set by the tracker and the API, never typed by hand, so the form leaves it out.

export const contactSchema = resourceFormSchema(
  zSiteUpdateContactInput
    .pick({
      email: true,
      phone: true,
      firstName: true,
      lastName: true,
      timeZone: true,
      country: true,
      locale: true,
      birthday: true,
      city: true,
      avatar: true,
      tags: true,
    })
    .required({ tags: true }),
  {
    email: '',
    phone: '',
    firstName: '',
    lastName: '',
    timeZone: '',
    country: '',
    locale: '',
    birthday: '',
    city: '',
    avatar: '',
    tags: [] as string[],
  },
)
