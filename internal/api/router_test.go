package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/marcelblijleven/bifrost/internal/api"
	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/sse"
)

// newFrontendStub stands in for the SvelteKit SSR server, echoing the
// request path so tests can assert what was proxied.
func newFrontendStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("frontend:" + r.URL.Path)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newSinglePortRouter(t *testing.T, frontend *httptest.Server) http.Handler {
	t.Helper()
	st := newHandlerMockStore()
	h := api.NewHandler(st, nil, pipeline.NewRegistry(), testJWTSecret, "", sse.New())
	u, err := url.Parse(frontend.URL)
	if err != nil {
		t.Fatalf("parse frontend url: %v", err)
	}
	return api.NewRouter(h, testAPIKey, testJWTSecret, u)
}

func TestSinglePortMode_UIPathsProxied(t *testing.T) {
	frontend := newFrontendStub(t)
	router := newSinglePortRouter(t, frontend)

	// Includes paths that collide with root-level API routes in split mode.
	for _, path := range []string{"/", "/login", "/applications", "/applications/123", "/_app/immutable/x.js"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "frontend:"+path {
			t.Errorf("GET %s = %d %q, want proxied to frontend", path, rec.Code, rec.Body.String())
		}
	}
}

func TestSinglePortMode_APIServedUnderPrefix(t *testing.T) {
	frontend := newFrontendStub(t)
	router := newSinglePortRouter(t, frontend)

	req := httptest.NewRequest(http.MethodGet, "/api/applications", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.HasPrefix(rec.Body.String(), "frontend:") {
		t.Errorf("GET /api/applications = %d %q, want JSON from the API", rec.Code, rec.Body.String())
	}
}

func TestSinglePortMode_RootEndpointsStayOnAPI(t *testing.T) {
	frontend := newFrontendStub(t)
	router := newSinglePortRouter(t, frontend)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || strings.HasPrefix(rec.Body.String(), "frontend:") {
		t.Errorf("GET /healthz = %d %q, want the API health response", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader("{}")))
	if strings.HasPrefix(rec.Body.String(), "frontend:") {
		t.Error("POST /webhooks/github must be handled by the API, not proxied")
	}
}

func TestSplitMode_APIServedAtRootAndPrefix(t *testing.T) {
	st := newHandlerMockStore()
	h := api.NewHandler(st, nil, pipeline.NewRegistry(), testJWTSecret, "", sse.New())
	router := api.NewRouter(h, testAPIKey, testJWTSecret, nil)

	for _, path := range []string{"/applications", "/api/applications"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+testAPIKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 in split mode", path, rec.Code)
		}
	}
}
