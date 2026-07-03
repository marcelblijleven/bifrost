package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// NewRouter builds the chi router with all routes and middleware applied.
//
// frontend enables single-port mode: when non-nil, non-API paths are
// reverse-proxied to the SvelteKit SSR server there and the API is served
// under /api only (root-level API paths collide with UI page paths). When
// nil, the API is additionally served at its historical root-level paths.
// Webhooks, health, and metrics stay at the root in both modes.
func NewRouter(h *Handler, apiKey, jwtSecret string, frontend *url.URL) http.Handler {
	r := chi.NewRouter()
	r.Use(RecoverMiddleware)
	r.Use(LoggingMiddleware)

	r.Get("/healthz", h.GetHealth)
	r.Handle("/metrics", promhttp.Handler())
	r.Post("/webhooks/{provider}", h.HandleWebhook)

	r.Route("/api", func(api chi.Router) {
		registerAPIRoutes(api, h, apiKey, jwtSecret)
	})

	if frontend == nil {
		registerAPIRoutes(r, h, apiKey, jwtSecret)
		return r
	}

	proxy := httputil.NewSingleHostReverseProxy(frontend)
	// Flush every write so proxied SSE streams reach the browser immediately.
	proxy.FlushInterval = -1
	r.NotFound(proxy.ServeHTTP)
	return r
}

func registerAPIRoutes(r chi.Router, h *Handler, apiKey, jwtSecret string) {
	r.Post("/auth/login", h.Login)
	r.Get("/setup", h.GetSetupStatus)
	r.Post("/setup", h.Setup)

	// Everything else requires a valid JWT or API key
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(apiKey, jwtSecret))

		r.Get("/auth/me", h.GetMe)
		r.Put("/auth/password", h.ChangePassword)
		r.Get("/dashboard", h.GetDashboard)
		r.Get("/providers", h.ListProviders)

		r.Get("/applications", h.ListApplications)
		r.Post("/applications", h.CreateApplication)
		r.Get("/applications/{id}", h.GetApplication)
		r.Put("/applications/{id}", h.UpdateApplication)
		r.Delete("/applications/{id}", h.DeleteApplication)
		r.Get("/applications/{id}/runs", h.ListRuns)
		r.Post("/applications/{id}/webhook/install", h.InstallWebhook)
		r.Post("/applications/{id}/head/accept", h.AcceptApplicationHead)
		r.Get("/applications/{id}/groups", h.ListApplicationGroups)
		r.Put("/applications/{id}/groups/{groupId}", h.GrantGroupAccess)
		r.Delete("/applications/{id}/groups/{groupId}", h.RevokeGroupAccess)

		r.Get("/runs/{id}", h.GetRun)
		r.Get("/runs/{id}/events", h.StreamRunEvents)
		r.Get("/runs/{id}/steps", h.ListStepResults)
		r.Post("/runs/{id}/steps/{stepIndex}/retry", h.RetryStep)
		r.Post("/runs/{id}/steps/{stepIndex}/override", h.OverrideStep)
		r.Post("/runs/{id}/cancel", h.CancelRun)
		r.Get("/runs/{id}/approvals", h.ListApprovals)
		r.Post("/runs/{id}/approvals/{stepIndex}/approve", h.ApproveStep)
		r.Post("/runs/{id}/approvals/{stepIndex}/reject", h.RejectStep)

		r.Get("/groups", h.ListGroups)
		r.Post("/groups", h.CreateGroup)
		r.Put("/groups/{id}", h.UpdateGroup)
		r.Delete("/groups/{id}", h.DeleteGroup)
		r.Get("/groups/{id}/members", h.ListGroupMembers)
		r.Put("/groups/{id}/members/{userId}", h.AddGroupMember)
		r.Delete("/groups/{id}/members/{userId}", h.RemoveGroupMember)

		r.Get("/users", h.ListUsers)
		r.Post("/users", h.CreateUser)
		r.Delete("/users/{id}", h.DeleteUser)
		r.Post("/users/{id}/password", h.ResetUserPassword)
		r.Put("/users/{id}/admin", h.SetUserAdmin)
	})
}
