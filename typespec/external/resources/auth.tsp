using TypeSpec.Http;

namespace Sphericon;

model ApiTokenInfo {
  id: EntityId;
  name: string;
  scopes: ApiTokenScope[];
  expiresAt?: Timestamp | null;
  revokedAt?: Timestamp | null;
  lastUsedAt?: Timestamp | null;
  createdAt: Timestamp;
  updatedAt: Timestamp;
}

model ApiTokenListResponse {
  items: ApiTokenInfo[];
}

model CreateApiTokenInput {
  name: string;
  scopes: ApiTokenScope[];
  expiresAt?: Timestamp | null;
}

model CreateApiTokenResponse {
  token: string;
  tokenInfo: ApiTokenInfo;
}

@route("/auth")
@tag("Auth")
namespace Auth {
  @post
  @route("/tokens")
  op tokensCreate(@body body: CreateApiTokenInput):
    | {
        @statusCode statusCode: 201;
        @body body: CreateApiTokenResponse;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | ForbiddenProblem
    | BadRequestProblem;

  @get
  @route("/tokens")
  op tokensList():
    | ApiTokenListResponse
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | ForbiddenProblem;

  @delete
  @route("/tokens/{id}")
  op tokensDelete(@path id: EntityId):
    | NoContentResponse
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | ForbiddenProblem
    | NotFoundProblem
    | BadRequestProblem;

  @get
  @route("/me")
  op meGet():
    | ApiTokenInfo
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | NotFoundProblem;

  @post
  @route("/tokens/bootstrap")
  @useAuth([])
  op tokensBootstrap(
    @header("X-Bootstrap-Token") bootstrapToken: string,
    @body body: CreateApiTokenInput,
  ):
    | {
        @statusCode statusCode: 201;
        @body body: CreateApiTokenResponse;
      }
    | UnauthorizedProblem
    | TooManyRequestsProblem
    | ForbiddenProblem
    | BadRequestProblem;
}
