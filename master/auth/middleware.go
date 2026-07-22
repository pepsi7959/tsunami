package auth

import (
	"context"
	"net/http"
)

type ctxKey struct{}

// publicPaths bypass the auth gate (login + the health/help stub). Everything
// else under the HTTP API requires a valid session cookie.
var publicPaths = map[string]bool{
	"/api/v1/login": true,
	"/help":         true,
}

// UserFrom returns the authenticated user stashed by Middleware, or nil.
func UserFrom(ctx context.Context) *User {
	u, _ := ctx.Value(ctxKey{}).(*User)
	return u
}

// cors sets credentialed CORS headers. With credentials the origin must be the
// exact request origin (never "*").
func cors(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Access-Control-Allow-Credentials", "true")
	h.Set("Vary", "Origin")
}

// Middleware gates every request behind a valid session cookie, letting CORS
// preflight and the public paths through. It runs before the ServeMux handlers,
// so it is the single choke-point for HTTP API auth.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cors(w, r)

		// CORS preflight: answer here, never require auth.
		if r.Method == http.MethodOptions {
			h := w.Header()
			h.Set("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE,OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Content-Length, X-Requested-With")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if publicPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		c, err := r.Cookie(a.cookieName)
		if err != nil || c.Value == "" {
			unauthorized(w)
			return
		}
		u, ok := a.Validate(c.Value)
		if !ok {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"code":40100,"data":null,"error":{"code":401,"message":"unauthorized"}}`))
}
