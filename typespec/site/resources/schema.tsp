using TypeSpec.Http;

namespace Sphericon;

/** The database schema of a development server (ADR 0033), which starts on a database that has
 * not run its migrations and answers every request 503 with code `migrations_pending` until
 * they run. */
@route("/schema")
@tag("Site Schema")
namespace SiteSchema {
  /** Run the migrations the database has not run, the button of the error page. Only a
   * development build serves it; any other answers 404. The answer comes once they ran. */
  @useAuth(NoAuth)
  @post
  @route("/migrations")
  op applyMigrations(): NoContentResponse | NotFoundProblem;
}
