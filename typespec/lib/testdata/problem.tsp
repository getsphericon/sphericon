// `@everyOperationDeclares("problem")`, as every surface but the Telemetry report declares it: every
// operation answers its errors with the one Problem, as its `default` response (ADR 0103).
import "@typespec/http";
import "../../common/scalars.tsp";
import "../../common/errors.tsp";

using TypeSpec.Http;
using Sphericon;

@service(#{ title: "Fixture" })
@everyOperationDeclares("problem")
namespace Fixture;

model Contact {
  name: string;
}

@route("/contacts")
@get
op contacts(): Contact[] | Problem;

@route("/tags")
@delete
op deleteTag(): NoContentResponse | Problem;

@route("/ping")
@post
op ping(): void | Problem;

// expect-next-line: sphericon/problem-missing
@route("/segments")
@get
op segments(): Contact[];

/** An error of its own, which ogen would make a member of a per-operation sum */
@error
model NotFound {
  @statusCode statusCode: 404;
  @body body: ProblemDetails;
}

// expect-next-line: sphericon/problem-missing
@route("/rules")
@get
op rules(): Contact[] | NotFound;

/** Another error model with no status code: a default response, but not the Problem */
@error
model Failure {
  @body body: ProblemDetails;
}

// expect-next-line: sphericon/problem-missing
@route("/automations")
@get
op automations(): Contact[] | Failure;

/** The Problem under a status of its own is no default response */
@error
model ProblemAt400 {
  @statusCode statusCode: 400;
  ...Problem;
}

// expect-next-line: sphericon/problem-missing
@route("/broadcasts")
@get
op broadcasts(): Contact[] | ProblemAt400;

/** A template's operation reports once, at the template, however many operations it becomes */
interface Reads<T> {
  // expect-next-line: sphericon/problem-missing
  @get
  list(): T[];

  @get
  @route("{id}")
  read(@path id: string): T | Problem;
}

@route("/events")
interface Events extends Reads<Contact> {}

@route("/visits")
interface Visits extends Reads<string> {}
