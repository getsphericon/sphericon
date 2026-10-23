export interface Course {
  slug: string
  title: string
  blurb: string
  // Fictional price in US dollars: the storefront has no payment provider.
  price: number
}

export const COURSES: readonly Course[] = [
  {
    slug: 'git-in-a-day',
    title: 'Git in a day',
    blurb: 'Branches, merges and rebases without the fear.',
    price: 29,
  },
  {
    slug: 'sql-for-analysts',
    title: 'SQL for analysts',
    blurb: 'Joins, windows and CTEs on real datasets.',
    price: 49,
  },
  {
    slug: 'design-systems-101',
    title: 'Design systems 101',
    blurb: 'Tokens, components and the rules that keep them honest.',
    price: 39,
  },
  {
    slug: 'python-for-everyone',
    title: 'Python for everyone',
    blurb: 'From your first script to a tool your team uses.',
    price: 59,
  },
]

export interface Plan {
  slug: string
  title: string
  price: number
  perks: readonly string[]
}

export const PLANS: readonly Plan[] = [
  { slug: 'free', title: 'Free', price: 0, perks: ['First lesson of every course'] },
  {
    slug: 'pro',
    title: 'Pro',
    price: 19,
    perks: ['Every course', 'Certificates', 'Offline lessons'],
  },
]
