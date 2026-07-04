package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/marcelblijleven/bifrost/internal/api"
	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/sse"
)

// uiStub stands in for the embedded frontend handler, echoing the request
// path so tests can assert which requests fell through to the UI.
func uiStub() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ui:" + r.URL.Path)) //nolint:errcheck
	})
}

func newUIRouter(t *testing.T) http.Handler {
	t.Helper()
	st := newHandlerMockStore()
	h := api.NewHandler(st, nil, pipeline.NewRegistry(), testJWTSecret, "", sse.New())
	return api.NewRouter(h, testAPIKey, testJWTSecret, uiStub())
}

func TestRouter_UIPathsFallThrough(t *testing.T) {
	router := newUIRouter(t)

	// Includes paths that collide with API resource names, which is why the
	// API lives under /api only.
	for _, path := range []string{"/", "/login", "/applications", "/applications/123", "/_app/immutable/x.js"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != "ui:"+path {
			t.Errorf("GET %s = %d %q, want served by the UI handler", path, rec.Code, rec.Body.String())
		}
	}
}

func TestRouter_APIServedUnderPrefix(t *testing.T) {
	router := newUIRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/applications", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.HasPrefix(rec.Body.String(), "ui:") {
		t.Errorf("GET /api/applications = %d %q, want JSON from the API", rec.Code, rec.Body.String())
	}
}

func TestRouter_RootEndpointsStayOnAPI(t *testing.T) {
	router := newUIRouter(t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || strings.HasPrefix(rec.Body.String(), "ui:") {
		t.Errorf("GET /healthz = %d %q, want the API health response", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader("{}")))
	if strings.HasPrefix(rec.Body.String(), "ui:") {
		t.Error("POST /webhooks/github must be handled by the API, not the UI")
	}
}

func TestRouter_NilUILeavesNonAPIUnrouted(t *testing.T) {
	st := newHandlerMockStore()
	h := api.NewHandler(st, nil, pipeline.NewRegistry(), testJWTSecret, "", sse.New())
	router := api.NewRouter(h, testAPIKey, testJWTSecret, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/applications", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("GET /api/applications = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/applications", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /applications = %d, want 404 without a UI handler", rec.Code)
	}
}
