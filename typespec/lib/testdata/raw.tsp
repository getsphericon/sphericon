// A raw @extension that writes a decorator's key is refused wherever it stands; any other
// extension passes.
import "@typespec/openapi";
import "../../common/scopes.tsp";

using TypeSpec.OpenAPI;
using Sphericon;

namespace Fixture;

model Contact {
  // expect-next-line: sphericon/raw-extension
  @extension("x-untrusted", true)
  name: string;

  @extension("x-something-else", true)
  email: string;
}

// expect-next-line: sphericon/raw-extension
@extension("x-required-scopes", #[ApiTokenScope.contactsRead])
op contacts(): Contact;

op tags(): void;

// expect-next-line: sphericon/raw-extension
@@extension(tags, "x-mcp", #{ hidden: true });

interface Reads<T> {
  // expect-next-line: sphericon/raw-extension
  @extension("x-required-permissions", #[])
  list(): T;
}

interface ContactReads extends Reads<Contact> {}
interface TagReads extends Reads<string> {}
