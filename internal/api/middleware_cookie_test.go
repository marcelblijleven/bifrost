package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/marcelblijleven/bifrost/internal/auth"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// sessionCookieName mirrors the unexported const in package api.
const sessionCookieName = "token"

func adminToken(t *testing.T) string {
	t.Helper()
	tok, err := auth.GenerateToken(uuid.New(), "admin@example.com", true, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return tok
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestLogin_SetsHttpOnlySessionCookie(t *testing.T) {
	st := newHandlerMockStore()
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	st.users["alice@example.com"] = &store.User{
		ID:           uuid.New(),
		Email:        "alice@example.com",
		PasswordHash: string(hash),
	}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "alice@example.com", "password": "hunter2"}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}

	c := findCookie(rr.Result().Cookies(), sessionCookieName)
	if c == nil {
		t.Fatal("login did not set a session cookie")
	}
	if c.Value == "" {
		t.Error("session cookie has empty value")
	}
	if !c.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict", c.SameSite)
	}
}

func TestLogout_ClearsSessionCookie(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodPost, "/auth/logout", nil, "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	c := findCookie(rr.Result().Cookies(), sessionCookieName)
	if c == nil {
		t.Fatal("logout did not set a clearing cookie")
	}
	if c.MaxAge >= 0 {
		t.Errorf("MaxAge = %d, want negative to expire the cookie", c.MaxAge)
	}
}

// cookieRequest builds a GET/POST to the API with the session cookie set and
// optional Origin / Sec-Fetch-Site headers.
func cookieRequest(method, path, token, origin, fetchSite string) *http.Request {
	req := httptest.NewRequest(method, "/api"+path, nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	return req
}

func TestCookieAuth_SafeMethodAllowed(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())

	// A GET authenticates from the cookie and is exempt from the CSRF check
	// even from a cross-site context.
	req := cookieRequest(http.MethodGet, "/auth/me", adminToken(t), "https://evil.example.com", "cross-site")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /auth/me with cookie = %d, want 200; body: %s", rr.Code, rr.Body)
	}
}

func TestCookieAuth_CrossOriginWriteRejected(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())

	// Sec-Fetch-Site: cross-site on a state-changing request is CSRF.
	req := cookieRequest(http.MethodPost, "/applications", adminToken(t), "", "cross-site")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST with cookie = %d, want 403", rr.Code)
	}

	// Fallback path: mismatched Origin with no Sec-Fetch-Site.
	req = cookieRequest(http.MethodPost, "/applications", adminToken(t), "https://evil.example.com", "")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST with cookie = %d, want 403", rr.Code)
	}
}

func TestCookieAuth_SameOriginWritePasses(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())

	// Same-origin passes the CSRF gate. The request reaches the handler,
	// which is what matters here: it must not be the 403 from the gate.
	req := cookieRequest(http.MethodPost, "/applications", adminToken(t), "", "same-origin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Fatalf("same-origin POST with cookie = 403, want the CSRF gate to pass")
	}
}

func TestBearerWrite_NotSubjectToCSRF(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())

	// Header auth is immune to CSRF: a cross-site POST must not be rejected
	// by the origin check just because it looks cross-site.
	req := httptest.NewRequest(http.MethodPost, "/api/applications", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Fatalf("cross-site POST with Bearer header = 403, want the CSRF gate skipped")
	}
}
