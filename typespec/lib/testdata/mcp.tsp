// @mcp: the compiler refuses an unknown key and a wrong type, the decorator the rest.
import "../main.tsp";

using Sphericon;

namespace Fixture;

op tool(): void;
op misspelled(): void;
op mistyped(): void;
op empty(): void;
op falseFlag(): void;
op hiddenNamed(): void;
op badName(): void;
op twice(): void;

@@mcp(tool, #{ name: "contacts_list", send: true });

// expect-next-line: invalid-argument
@@mcp(misspelled, #{ hide: true });

// expect-next-line: invalid-argument
@@mcp(mistyped, #{ hidden: "true" });

// expect-next-line: sphericon/mcp-empty
@@mcp(empty, #{});

// expect-next-line: sphericon/mcp-false
@@mcp(falseFlag, #{ send: false });

// expect-next-line: sphericon/mcp-hidden-tool
@@mcp(hiddenNamed, #{ hidden: true, name: "x" });

// expect-next-line: sphericon/mcp-name
@@mcp(badName, #{ name: "Contacts-List" });

@@mcp(twice, #{ hidden: true });

// expect-next-line: sphericon/declared-twice
@@mcp(twice, #{ send: true });
