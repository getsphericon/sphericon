// The Custom field models /site and /api show and take the same way.

namespace Sphericon;

/** A Custom field's value type */
enum CustomFieldType {
  string,
  number,
  bool,
  datetime,
}

/** Request body for updating a custom field: its key never changes */
model UpdateCustomFieldInput {
  /** The custom field's `revision` the edit was made on (ADR 0119): the one it was read with */
  revision: Revision;

  /** Display name */
  name?: Name;

  /** Value type. Stored values are not rewritten: the leaves of the new type read them as they are */
  type?: CustomFieldType;
}
