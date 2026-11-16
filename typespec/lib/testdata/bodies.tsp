// `@closedRequestBodies`, as /api, /site and /staff declare it, and `@openMap` (ADR 0148). The
// surface is versioned like /api, whose document is emitted from a copy of every type.
import "@typespec/http";
import "@typespec/versioning";
import "../main.tsp";

using TypeSpec.Http;
using TypeSpec.Versioning;
using Sphericon;

@service(#{ title: "Fixture" })
@versioned(Versions)
@closedRequestBodies
namespace Fixture;

enum Versions {
  v1,
}

/** A map whose keys are data */
@openMap
model Properties is Record<unknown>;

/** A leaf nested in a union */
model Leaf {
  field: string;
}

/** Another variant */
model Group {
  leaves: Leaf[];
}

model Content {
  subject: string;
  condition?: Leaf | Group;
  properties?: Properties;
}

model Input {
  name: string;
  content: Content;
}

/** Read only: it is never in a request body */
model Output {
  name: string;
  extra: Record<string>;
}

@route("/inputs")
@post
op create(@body body: Input): Output;

model Spread {
  name: string;
}

@route("/spread")
@post
op spread(...Spread): void;

model Tolerant is Record<string>;

model Leaky {
  // expect-next-line: sphericon/open-in-request-body
  tolerant: Tolerant;

  // expect-next-line: sphericon/anonymous-in-request-body
  inline: {
    a: string;
  };
}

@route("/leaky")
@post
op leaky(@body body: Leaky): void;

model Base {
  a: string;
}

// expect-next-line: sphericon/extends-in-request-body
model Derived extends Base {
  b: string;
}

@route("/derived")
@post
op derived(@body body: Derived): void;

// expect-next-line: sphericon/closed-in-response
model Shared {
  a: string;
}

model WithShared {
  shared: Shared;
}

@route("/shared")
@post
op shared(@body body: WithShared): Shared[];
