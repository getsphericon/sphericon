---
status: accepted
---

# A Form submit binds only an anonymous Visitor and only fills a known Contact

An On-site form (ADR 0085) captures people on the customer's site through the tracker. Identify
already binds a Visitor to a Contact and writes its attributes (ADR 0002, ADR 0072): it resolves the
Contact by subject_id, email or phone, moves the Visitor to it even when the Visitor belonged to
another Contact, and overwrites the core attributes it is given. That suits a call made by the
customer's own code after a sign-in. A form is typed by anyone: on a shared computer, or on purpose,
a visitor can enter someone else's email. Decided in the submit ticket of the on-site map
(getsphericon/sphericon#953).

## Decisions

- **A submit binds only an anonymous Visitor.** It finds or creates the Contact by the email, or
  by the phone when there is no email, and binds the Visitor to it only while the Visitor belongs to
  no Contact, so the browsing before it is stitched on, as Klaviyo does. A Visitor that belongs to
  another Contact stays where it is, with its web push Binding and its Segment targeting; the submit
  and its Event go to the Contact the form named.
- **A submit only fills a known Contact.** On a Contact that existed it writes only empty
  attributes; a Contact it creates gets every field. Otherwise anyone could rename a stranger in the
  mail sent to them. A phone that belongs to another Contact is not attached, and the submit still
  succeeds.
- **The form's fields are fixed at authoring.** Each maps to a core attribute, an alias key or a
  Custom field chosen (or created) by the author; a submit through the public collect key never
  creates a Custom field. Every value is also kept in the Event's properties, so an anonymous
  answer reaches the Contact when the Visitor is identified later.
- **One Event per filling, and no other record.** `onsite_form.submitted` is recorded once, at the
  step that names the person (answers of earlier steps travel with it); later steps only write
  attributes. A form that names no one records it at its last step. With `onsite_form.shown`,
  `.closed` and `.clicked` it is a System event under the reserved `onsite_form.` prefix, on the
  Visitor and its Contact. A Contact a form creates has the Contact source `onsite_form`.
- **The form acts on nothing else.** Tags, an enrollment or an HTTP call follow from a Rule whose
  Condition names the form; an Automation may be triggered by `onsite_form.submitted` of any form.

## Considered options

- **A submit is an Identify.** One path, but it hands a stranger's Visitor and attributes to
  whoever types their email. Customer.io goes the other way and does not identify the browser at
  all; that loses the stitching that makes a capture form worth having.
- **Actions in the form** (apply a Tag, enroll), as ActiveCampaign and Drip offer. Rejected: a
  second action model beside the Rule's.
