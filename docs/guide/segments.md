# Segments

A segment is a named, reusable rule that answers "which contacts match?". It is the one way to
choose who receives a broadcast.

## Build a rule

In **Segments**, combine conditions with all/any and nest groups where needed. A condition can
look at:

- **Attributes**: email, name, time zone and the other core fields.
- **Custom fields**: any field your tracking or import created, with the operators its type
  supports. A rule that names a field your workspace does not have is rejected when you save
  the segment.
- **Tags**: whether the contact has a tag or not.
- **Events**: whether the contact did, or did not do, something, optionally within the last
  N days. Because delivery facts are events, you can target on `email.opened` or `email.clicked`
  the same way.

A live preview shows how many contacts match as you edit.

## Evaluated live

Membership is never stored. A segment is a query that runs when you preview it and when a
broadcast starts sending, so it reflects your data at that moment.

There is deliberately no list or "subscribed to X" group. Tags and custom fields describe a
contact; a segment decides who to target; and the opt-out rules described in
[Deliverability and consent](/guide/deliverability) always subtract from the result.

## Segments through the API

The definition is a rule group: a `combinator` (`and` or `or`), an optional `not`, and `rules`,
each a rule or a nested group, told apart by `kind`. A rule has a `field`, an `operator` and a
`value`. An empty group matches every contact.

| Field                 | Operators                                                | Value                          |
| --------------------- | -------------------------------------------------------- | ------------------------------ |
| `email`, `phone`, ... | `eq`, `neq`, `contains`, `beginsWith`, `null`, `notNull` | the text to compare            |
| `custom:<key>`        | `eq`, `neq`, `contains`, `notNull`                       | the value to compare           |
| `tag`                 | `has`, `doesNotHave`                                     | the tag name                   |
| `event:<action>`      | `performed`, `notPerformed`                              | a window in days, blank = ever |

```sh
curl https://sphericon.example.com/api/segments/preview \
  -H "Authorization: Bearer $SPHERICON_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"definition":{"kind":"group","combinator":"and","rules":[
        {"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"},
        {"kind":"rule","field":"event:signed_up","operator":"performed","value":"30"}]}}'
```
