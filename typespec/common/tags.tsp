// The Tag write bodies: /site and /api take the same input.

namespace Sphericon;

/** Request body for applying a tag to a contact */
model ApplyTagInput {
  /** The label to apply; created on first use */
  name: string;
}
