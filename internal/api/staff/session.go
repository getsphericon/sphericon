package staff

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/getsphericon/sphericon/internal/staffmembers"
)

// RequireSession serves next only to a request that carries a valid Staff member session
// (the same cookie and check as the contract's security handler) and answers any other with
// 401. It guards the /staff pages outside the contract, such as the job queue's UI, which the
// ogen server's security handler never sees.
func RequireSession(sessions *staffmembers.Sessions, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(staffmembers.SessionCookie)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		ctx, err := sessions.Authenticate(r.Context(), cookie.Value)
		if errors.Is(err, staffmembers.ErrUnauthorized) {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		if err != nil {
			slog.ErrorContext(r.Context(), "staff session check failed", "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
