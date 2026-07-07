package api

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/marcelblijleven/bifrost/internal/auth"
)

// sessionCookie is the httpOnly cookie carrying the browser session JWT.
// The name predates the single-binary setup (the SvelteKit server used the
// same one), so existing sessions keep working across the migration.
const sessionCookie = "token"

// LoggingMiddleware logs method, path, status, and duration for every request.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"duration", time.Since(start),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(status int) {
	sw.status = status
	sw.ResponseWriter.WriteHeader(status)
}

// APIKeyMiddleware enforces "Authorization: Bearer <key>" on protected routes.
func APIKeyMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != key {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware accepts either a valid JWT or the static API key, taken
// from the Authorization header or (for browser sessions) the session cookie.
// For JWTs it stores the claims in the request context.
func AuthMiddleware(apiKey, jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, fromCookie := credentials(r)
			if token == "" {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			// Cookies are attached by the browser to cross-site requests
			// too, so cookie-authenticated writes need a same-origin check.
			// Header-based auth is immune to CSRF and skips it.
			if fromCookie && !sameOriginRequest(r) {
				writeError(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}

			// Static API key — no user identity attached
			if token == apiKey {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := auth.ValidateToken(token, jwtSecret)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
		})
	}
}

// credentials extracts the bearer token from the Authorization header or,
// failing that, the session cookie. fromCookie reports which source was used.
func credentials(r *http.Request) (token string, fromCookie bool) {
	if raw := r.Header.Get("Authorization"); strings.HasPrefix(raw, "Bearer ") {
		return strings.TrimPrefix(raw, "Bearer "), false
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value, true
	}
	return "", false
}

// sameOriginRequest reports whether a state-changing request originated from
// our own frontend. Modern browsers send Sec-Fetch-Site on every request and
// Origin on every non-GET request; a mismatch on either means CSRF.
func sameOriginRequest(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	// "none" is a direct navigation (e.g. address bar), not an attack vector
	// for fetch/XHR-style requests.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	// Backstop for browsers without Sec-Fetch-Site. "null" (sandboxed
	// iframes, some redirects) is deliberately rejected.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			return false
		}
	}
	return true
}

// requestIsSecure reports whether the browser reached this server over HTTPS,
// from the actual connection or a TLS-terminating proxy's X-Forwarded-Proto,
// so the Secure cookie flag matches the scheme the browser used.
func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		if i := strings.IndexByte(proto, ','); i >= 0 {
			proto = proto[:i]
		}
		return strings.EqualFold(strings.TrimSpace(proto), "https")
	}
	return false
}

// RecoverMiddleware catches panics, logs them, and returns a 500.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "err", err, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
