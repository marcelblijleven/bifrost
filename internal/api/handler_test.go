package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/marcelblijleven/bifrost/internal/api"
	"github.com/marcelblijleven/bifrost/internal/auth"
	"github.com/marcelblijleven/bifrost/internal/pipeline"
	"github.com/marcelblijleven/bifrost/internal/pipeline/steps"
	"github.com/marcelblijleven/bifrost/internal/provider"
	"github.com/marcelblijleven/bifrost/internal/store"
)

// ── mock store ────────────────────────────────────────────────────────────────

type handlerMockStore struct {
	applications    map[uuid.UUID]*store.Application
	runs            map[uuid.UUID]*store.PipelineRun
	steps           map[uuid.UUID][]*store.StepResult
	users           map[string]*store.User
	approvals       map[uuid.UUID]*store.ApprovalRequest
	pendingApproval map[string]*store.ApprovalRequest // key: "runID:stepIndex"
}

func newHandlerMockStore() *handlerMockStore {
	return &handlerMockStore{
		applications:    make(map[uuid.UUID]*store.Application),
		runs:            make(map[uuid.UUID]*store.PipelineRun),
		steps:           make(map[uuid.UUID][]*store.StepResult),
		users:           make(map[string]*store.User),
		approvals:       make(map[uuid.UUID]*store.ApprovalRequest),
		pendingApproval: make(map[string]*store.ApprovalRequest),
	}
}

func (m *handlerMockStore) CreateApplication(_ context.Context, a *store.Application) error {
	a.ID = uuid.New()
	m.applications[a.ID] = a
	return nil
}
func (m *handlerMockStore) GetApplication(_ context.Context, id uuid.UUID) (*store.Application, error) {
	if a, ok := m.applications[id]; ok {
		return a, nil
	}
	return nil, errors.New("not found")
}
func (m *handlerMockStore) ListApplicationsByRepo(_ context.Context, provider, owner, repo string) ([]*store.Application, error) {
	out := make([]*store.Application, 0)
	for _, a := range m.applications {
		if a.Provider == provider && a.Owner == owner && a.Repo == repo {
			out = append(out, a)
		}
	}
	return out, nil
}
func (m *handlerMockStore) GetRunByTriggerTag(_ context.Context, applicationID uuid.UUID, tag string) (*store.PipelineRun, error) {
	for _, r := range m.runs {
		if r.ApplicationID == applicationID && r.TriggerTag == tag {
			return r, nil
		}
	}
	return nil, nil
}
func (m *handlerMockStore) ListApplications(_ context.Context) ([]*store.Application, error) {
	out := make([]*store.Application, 0, len(m.applications))
	for _, a := range m.applications {
		out = append(out, a)
	}
	return out, nil
}
func (m *handlerMockStore) ListApplicationsForUser(_ context.Context, _ uuid.UUID) ([]*store.Application, error) {
	out := make([]*store.Application, 0, len(m.applications))
	for _, a := range m.applications {
		out = append(out, a)
	}
	return out, nil
}
func (m *handlerMockStore) CanUserAccessApplication(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return true, nil
}
func (m *handlerMockStore) UpdateApplication(_ context.Context, a *store.Application) error {
	m.applications[a.ID] = a
	return nil
}
func (m *handlerMockStore) DeleteApplication(_ context.Context, id uuid.UUID) error {
	delete(m.applications, id)
	return nil
}
func (m *handlerMockStore) ListLatestRuns(_ context.Context) (map[uuid.UUID]*store.PipelineRun, error) {
	latest := make(map[uuid.UUID]*store.PipelineRun)
	for _, r := range m.runs {
		if cur, ok := latest[r.ApplicationID]; !ok || r.CreatedAt.After(cur.CreatedAt) {
			latest[r.ApplicationID] = r
		}
	}
	return latest, nil
}
func (m *handlerMockStore) AdvanceApplicationHead(_ context.Context, id uuid.UUID, from, to string) (bool, error) {
	a, ok := m.applications[id]
	if !ok || a.LastKnownSHA != from || a.HeadState == store.HeadStateBlocked {
		return false, nil
	}
	a.LastKnownSHA = to
	return true, nil
}
func (m *handlerMockStore) BlockApplication(_ context.Context, id uuid.UUID, reason string) error {
	a, ok := m.applications[id]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now()
	a.HeadState = store.HeadStateBlocked
	a.BlockedReason = reason
	a.BlockedAt = &now
	return nil
}
func (m *handlerMockStore) AcceptApplicationHead(_ context.Context, id uuid.UUID, head string) error {
	a, ok := m.applications[id]
	if !ok {
		return errors.New("not found")
	}
	a.HeadState = store.HeadStateOK
	a.LastKnownSHA = head
	a.BlockedReason = ""
	a.BlockedAt = nil
	return nil
}
func (m *handlerMockStore) CancelPendingRuns(_ context.Context, applicationID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, r := range m.runs {
		if r.ApplicationID == applicationID && r.Status == "pending" {
			r.Status = "cancelled"
			ids = append(ids, r.ID)
		}
	}
	return ids, nil
}
func (m *handlerMockStore) GrantGroupAccess(_ context.Context, _, _ uuid.UUID) error  { return nil }
func (m *handlerMockStore) RevokeGroupAccess(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *handlerMockStore) ListApplicationGroups(_ context.Context, _ uuid.UUID) ([]*store.Group, error) {
	return make([]*store.Group, 0), nil
}
func (m *handlerMockStore) CreatePipelineRun(_ context.Context, r *store.PipelineRun) error {
	m.runs[r.ID] = r
	return nil
}
func (m *handlerMockStore) GetPipelineRun(_ context.Context, id uuid.UUID) (*store.PipelineRun, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return nil, errors.New("not found")
}
func (m *handlerMockStore) ListPipelineRuns(_ context.Context, appID uuid.UUID, _, _ int, _ store.RunFilter) ([]*store.PipelineRun, error) {
	out := make([]*store.PipelineRun, 0)
	for _, r := range m.runs {
		if r.ApplicationID == appID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *handlerMockStore) UpdatePipelineRun(_ context.Context, r *store.PipelineRun) error {
	if existing, ok := m.runs[r.ID]; ok {
		if r.Status != "" {
			existing.Status = r.Status
		}
	}
	return nil
}
func (m *handlerMockStore) CreateStepResult(_ context.Context, r *store.StepResult) error {
	m.steps[r.RunID] = append(m.steps[r.RunID], r)
	return nil
}
func (m *handlerMockStore) UpdateStepResult(_ context.Context, _ *store.StepResult) error {
	return nil
}
func (m *handlerMockStore) ListStepResults(_ context.Context, runID uuid.UUID) ([]*store.StepResult, error) {
	return m.steps[runID], nil
}
func (m *handlerMockStore) DeleteStepResultsFrom(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}
func (m *handlerMockStore) CreateUser(_ context.Context, u *store.User) error {
	u.ID = uuid.New()
	m.users[u.Email] = u
	return nil
}
func (m *handlerMockStore) GetUserByEmail(_ context.Context, email string) (*store.User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}
func (m *handlerMockStore) GetUserForAuth(_ context.Context, email string) (*store.User, error) {
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}
func (m *handlerMockStore) ListUsers(_ context.Context) ([]*store.User, error) {
	out := make([]*store.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	return out, nil
}
func (m *handlerMockStore) CountAdmins(_ context.Context) (int, error) {
	n := 0
	for _, u := range m.users {
		if u.IsAdmin {
			n++
		}
	}
	return n, nil
}
func (m *handlerMockStore) UpdateUserPassword(_ context.Context, id uuid.UUID, passwordHash string) error {
	for _, u := range m.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			return nil
		}
	}
	return errors.New("not found")
}
func (m *handlerMockStore) SetUserAdmin(_ context.Context, id uuid.UUID, isAdmin bool) error {
	for _, u := range m.users {
		if u.ID == id {
			u.IsAdmin = isAdmin
			return nil
		}
	}
	return errors.New("not found")
}
func (m *handlerMockStore) DeleteUser(_ context.Context, id uuid.UUID) error {
	for email, u := range m.users {
		if u.ID == id {
			delete(m.users, email)
			return nil
		}
	}
	return errors.New("not found")
}
func (m *handlerMockStore) CreateGroup(_ context.Context, g *store.Group) error {
	g.ID = uuid.New()
	return nil
}
func (m *handlerMockStore) GetGroup(_ context.Context, _ uuid.UUID) (*store.Group, error) {
	return nil, errors.New("not found")
}
func (m *handlerMockStore) ListGroups(_ context.Context) ([]*store.Group, error) {
	return make([]*store.Group, 0), nil
}
func (m *handlerMockStore) UpdateGroup(_ context.Context, _ *store.Group) error         { return nil }
func (m *handlerMockStore) DeleteGroup(_ context.Context, _ uuid.UUID) error            { return nil }
func (m *handlerMockStore) AddUserToGroup(_ context.Context, _, _ uuid.UUID) error      { return nil }
func (m *handlerMockStore) RemoveUserFromGroup(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *handlerMockStore) ListGroupMembers(_ context.Context, _ uuid.UUID) ([]*store.User, error) {
	return make([]*store.User, 0), nil
}
func (m *handlerMockStore) CreateApprovalRequest(_ context.Context, r *store.ApprovalRequest) error {
	m.approvals[r.ID] = r
	return nil
}
func (m *handlerMockStore) GetApprovalRequest(_ context.Context, id uuid.UUID) (*store.ApprovalRequest, error) {
	if r, ok := m.approvals[id]; ok {
		return r, nil
	}
	return nil, errors.New("not found")
}
func (m *handlerMockStore) GetPendingApproval(_ context.Context, _ uuid.UUID, _ int) (*store.ApprovalRequest, error) {
	return nil, errors.New("not found")
}
func (m *handlerMockStore) GetApprovalForStep(_ context.Context, _ uuid.UUID, _ int) (*store.ApprovalRequest, error) {
	return nil, errors.New("not found")
}
func (m *handlerMockStore) DeleteApprovalRequestsFrom(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}
func (m *handlerMockStore) ResolveApprovalRequest(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (m *handlerMockStore) SupersedeOlderApprovals(_ context.Context, _ uuid.UUID, _ int, _ uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}
func (m *handlerMockStore) ListApprovalRequests(_ context.Context, _ uuid.UUID) ([]*store.ApprovalRequest, error) {
	return make([]*store.ApprovalRequest, 0), nil
}
func (m *handlerMockStore) GetStepResultByExternalRunID(_ context.Context, _ int64) (*store.StepResult, error) {
	return nil, nil
}
func (m *handlerMockStore) ClaimPendingRun(_ context.Context, _ string) (*store.PipelineRun, error) {
	return nil, nil
}
func (m *handlerMockStore) HeartbeatRun(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return true, nil
}
func (m *handlerMockStore) ReapExpiredRuns(_ context.Context) (int, error) { return 0, nil }
func (m *handlerMockStore) OverrideStepResult(_ context.Context, runID uuid.UUID, stepIndex int, by, reason string) (bool, error) {
	for _, s := range m.steps[runID] {
		if s.StepIndex == stepIndex && s.Status == "failed" {
			now := time.Now()
			s.Status = "overridden"
			s.OverriddenBy = by
			s.OverrideReason = reason
			s.OverriddenAt = &now
			return true, nil
		}
	}
	return false, nil
}
func (m *handlerMockStore) SetStepResultExternalRunID(_ context.Context, runID uuid.UUID, stepIndex int, externalRunID int64) error {
	for _, s := range m.steps[runID] {
		if s.StepIndex == stepIndex {
			s.ExternalRunID = &externalRunID
		}
	}
	return nil
}
func (m *handlerMockStore) UpdateRunTag(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}
func (m *handlerMockStore) MarkRunReleased(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *handlerMockStore) GetLastReleasedRun(_ context.Context, _ uuid.UUID, _ string) (*store.PipelineRun, error) {
	return nil, nil
}
func (m *handlerMockStore) ResetStepResultsFrom(_ context.Context, _ uuid.UUID, _ int) error {
	return nil
}
func (m *handlerMockStore) CancelRun(_ context.Context, _ uuid.UUID) error { return nil }
func (m *handlerMockStore) ResetRunToPending(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *handlerMockStore) GetDashboardStats(_ context.Context) (*store.DashboardStats, error) {
	return &store.DashboardStats{
		RunsByDay:  make([]store.DayStats, 0),
		RecentRuns: make([]store.RecentRun, 0),
	}, nil
}
func (m *handlerMockStore) Close() {}

// ── test helpers ──────────────────────────────────────────────────────────────

const (
	testJWTSecret = "handler-test-secret"
	testAPIKey    = "test-api-key"
)

func newTestRouter(st store.Store) http.Handler {
	return newTestRouterWithProviders(st, nil)
}

func newTestRouterWithProviders(st store.Store, providers map[string]provider.Provider) http.Handler {
	reg := pipeline.NewRegistry()
	h := api.NewHandler(st, providers, reg, testJWTSecret, "", nil)
	return api.NewRouter(h, testAPIKey, testJWTSecret)
}

// newTestRouterWithSteps registers the real step types (mirroring cmd/bifrost/main.go)
// so pipeline ordering validation can be exercised.
func newTestRouterWithSteps(st store.Store) http.Handler {
	reg := pipeline.NewRegistry()
	reg.Register("semver", steps.NewSemverStep)
	reg.Register("tag", func(map[string]any) (pipeline.Step, error) {
		return &steps.TagStep{}, nil
	})
	reg.Register("changelog", func(map[string]any) (pipeline.Step, error) {
		return &steps.ChangelogStep{}, nil
	})
	reg.Register("create_release", steps.NewCreateReleaseStep)
	reg.Register("notify", steps.NewNotifyStep)
	h := api.NewHandler(st, nil, reg, testJWTSecret, "", nil)
	return api.NewRouter(h, testAPIKey, testJWTSecret)
}

func authHeader(t *testing.T) string {
	t.Helper()
	token, err := auth.GenerateToken(uuid.New(), "test@example.com", true, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return "Bearer " + token
}

func doRequest(t *testing.T, handler http.Handler, method, path string, body any, authHdr string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if authHdr != "" {
		req.Header.Set("Authorization", authHdr)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rr.Body).Decode(v); err != nil {
		t.Fatalf("decode response JSON: %v", err)
	}
}

// ── auth tests ────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	st := newHandlerMockStore()
	password := "hunter2"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	st.users["alice@example.com"] = &store.User{
		ID:           uuid.New(),
		Email:        "alice@example.com",
		PasswordHash: string(hash),
	}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "alice@example.com", "password": password}, "")

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var resp map[string]string
	decodeJSON(t, rr, &resp)
	if resp["token"] == "" {
		t.Error("expected non-empty token in response")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	st := newHandlerMockStore()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	st.users["bob@example.com"] = &store.User{
		ID:           uuid.New(),
		Email:        "bob@example.com",
		PasswordHash: string(hash),
	}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "bob@example.com", "password": "wrong"}, "")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestLogin_RateLimitedAfterTooManyFailures(t *testing.T) {
	st := newHandlerMockStore()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	st.users["carol@example.com"] = &store.User{
		ID:           uuid.New(),
		Email:        "carol@example.com",
		PasswordHash: string(hash),
	}
	router := newTestRouter(st)

	for i := 0; i < 5; i++ {
		rr := doRequest(t, router, http.MethodPost, "/auth/login",
			map[string]string{"email": "carol@example.com", "password": "wrong"}, "")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, rr.Code)
		}
	}

	// Locked out now, even with the correct password.
	rr := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "carol@example.com", "password": "correct"}, "")
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body: %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestLogin_SuccessResetsRateLimit(t *testing.T) {
	st := newHandlerMockStore()
	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.MinCost)
	st.users["dave@example.com"] = &store.User{
		ID:           uuid.New(),
		Email:        "dave@example.com",
		PasswordHash: string(hash),
	}
	router := newTestRouter(st)

	for i := 0; i < 3; i++ {
		doRequest(t, router, http.MethodPost, "/auth/login",
			map[string]string{"email": "dave@example.com", "password": "wrong"}, "")
	}
	rr := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "dave@example.com", "password": "correct"}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}

	// Failure count should have reset; two more failures shouldn't lock out
	// (would need 5 to trip the limiter fresh).
	for i := 0; i < 2; i++ {
		doRequest(t, router, http.MethodPost, "/auth/login",
			map[string]string{"email": "dave@example.com", "password": "wrong"}, "")
	}
	rr = doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "dave@example.com", "password": "correct"}, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status after reset = %d, want 200; body: %s", rr.Code, rr.Body)
	}
}

func TestLogin_UnknownUser(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": "nobody@example.com", "password": "x"}, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestGetMe_WithJWT(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/auth/me", nil, authHeader(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var resp map[string]any
	decodeJSON(t, rr, &resp)
	if resp["email"] == "" {
		t.Error("expected email in /auth/me response")
	}
}

func TestGetMe_NoAuth(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/auth/me", nil, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestGetMe_WithAPIKey(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/auth/me", nil, "Bearer "+testAPIKey)
	// API key auth succeeds but GetMe can't find claims → 401
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (no JWT claims with API key)", rr.Code)
	}
}

// ── application CRUD tests ────────────────────────────────────────────────────

func TestListApplications_Empty(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/applications", nil, authHeader(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	var apps []store.Application
	decodeJSON(t, rr, &apps)
	if len(apps) != 0 {
		t.Errorf("expected empty list, got %d items", len(apps))
	}
}

func TestCreateAndGetApplication(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouter(st)
	hdr := authHeader(t)

	body := map[string]any{
		"Name":     "my-app",
		"Provider": "github",
		"Owner":    "acme",
		"Repo":     "widget",
		"Branch":   "main",
	}
	rr := doRequest(t, router, http.MethodPost, "/applications", body, hdr)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rr.Code, rr.Body)
	}
	var created store.Application
	decodeJSON(t, rr, &created)
	if created.Name != "my-app" {
		t.Errorf("Name = %q, want %q", created.Name, "my-app")
	}

	// GET by ID
	rr = doRequest(t, router, http.MethodGet, "/applications/"+created.ID.String(), nil, hdr)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var fetched store.Application
	decodeJSON(t, rr, &fetched)
	if fetched.ID != created.ID {
		t.Errorf("fetched ID = %v, want %v", fetched.ID, created.ID)
	}
}

func TestCreateApplication_RejectsInvalidPipelineOrder(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouterWithSteps(st)
	hdr := authHeader(t)

	body := map[string]any{
		"Name":     "my-app",
		"Provider": "github",
		"Owner":    "acme",
		"Repo":     "widget",
		"Branch":   "main",
		"PipelineSteps": []map[string]any{
			{"type": "tag"},
			{"type": "semver"},
		},
	}
	rr := doRequest(t, router, http.MethodPost, "/applications", body, hdr)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body)
	}
}

func TestCreateApplication_AcceptsValidPipelineOrder(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouterWithSteps(st)
	hdr := authHeader(t)

	body := map[string]any{
		"Name":     "my-app",
		"Provider": "github",
		"Owner":    "acme",
		"Repo":     "widget",
		"Branch":   "main",
		"PipelineSteps": []map[string]any{
			{"type": "semver"},
			{"type": "changelog"},
			{"type": "tag"},
			{"type": "create_release"},
			{"type": "notify", "config": map[string]any{"url": "https://example.com/hook"}},
		},
	}
	rr := doRequest(t, router, http.MethodPost, "/applications", body, hdr)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body)
	}
}

func TestUpdateApplication_RejectsInvalidPipelineOrder(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouterWithSteps(st)
	hdr := authHeader(t)

	id := uuid.New()
	st.applications[id] = &store.Application{ID: id, Name: "my-app"}

	body := map[string]any{
		"Name": "my-app",
		"PipelineSteps": []map[string]any{
			{"type": "create_release"},
			{"type": "semver"},
		},
	}
	rr := doRequest(t, router, http.MethodPut, "/applications/"+id.String(), body, hdr)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body)
	}
}

func TestGetApplication_NotFound(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/applications/"+uuid.New().String(), nil, authHeader(t))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestDeleteApplication(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouter(st)
	hdr := authHeader(t)

	app := &store.Application{ID: uuid.New(), Name: "to-delete"}
	st.applications[app.ID] = app

	rr := doRequest(t, router, http.MethodDelete, "/applications/"+app.ID.String(), nil, hdr)
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rr.Code)
	}
	if _, ok := st.applications[app.ID]; ok {
		t.Error("application was not deleted from store")
	}
}

// ── run tests ─────────────────────────────────────────────────────────────────

func TestGetRun_Found(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouter(st)

	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{
		ID:     runID,
		Status: "success",
		Branch: "main",
	}

	rr := doRequest(t, router, http.MethodGet, "/runs/"+runID.String(), nil, authHeader(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var run store.PipelineRun
	decodeJSON(t, rr, &run)
	if run.Status != "success" {
		t.Errorf("Status = %q, want %q", run.Status, "success")
	}
}

func TestGetRun_NotFound(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/runs/"+uuid.New().String(), nil, authHeader(t))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestListRuns(t *testing.T) {
	st := newHandlerMockStore()
	appID := uuid.New()
	st.applications[appID] = &store.Application{ID: appID, Name: "app"}
	for range 3 {
		id := uuid.New()
		st.runs[id] = &store.PipelineRun{ID: id, ApplicationID: appID, Status: "success"}
	}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodGet, "/applications/"+appID.String()+"/runs", nil, authHeader(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var runs []*store.PipelineRun
	decodeJSON(t, rr, &runs)
	if len(runs) != 3 {
		t.Errorf("got %d runs, want 3", len(runs))
	}
}

// ── retry step tests ──────────────────────────────────────────────────────────

func TestRetryStep_RunNotFound(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+uuid.New().String()+"/steps/0/retry", nil, authHeader(t))
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestRetryStep_RunNotFailed(t *testing.T) {
	st := newHandlerMockStore()
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, Status: "success"}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+runID.String()+"/steps/0/retry", nil, authHeader(t))
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestRetryStep_StepNotFailed(t *testing.T) {
	st := newHandlerMockStore()
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, Status: "failed"}
	st.steps[runID] = []*store.StepResult{
		{ID: uuid.New(), RunID: runID, StepIndex: 0, Status: "success"},
	}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+runID.String()+"/steps/0/retry", nil, authHeader(t))
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestRetryStep_CancelledRunAllowed(t *testing.T) {
	st := newHandlerMockStore()
	appID := uuid.New()
	st.applications[appID] = &store.Application{ID: appID, Provider: "github"}
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, ApplicationID: appID, Status: "cancelled"}
	st.steps[runID] = []*store.StepResult{
		{ID: uuid.New(), RunID: runID, StepIndex: 0, Status: "cancelled"},
	}

	// Provider must be configured for the retry to be accepted.
	router := newTestRouterWithProviders(st, map[string]provider.Provider{"github": nil})
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+runID.String()+"/steps/0/retry", nil, authHeader(t))
	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202; body: %s", rr.Code, rr.Body)
	}
}

// ── providers endpoint ────────────────────────────────────────────────────────

func TestListProviders(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouterWithProviders(st, map[string]provider.Provider{
		"github": nil,
		"gitea":  nil,
	})
	rr := doRequest(t, router, http.MethodGet, "/providers", nil, authHeader(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var resp struct {
		Providers []string `json:"providers"`
	}
	decodeJSON(t, rr, &resp)
	if len(resp.Providers) != 2 || resp.Providers[0] != "gitea" || resp.Providers[1] != "github" {
		t.Errorf("providers = %v, want [gitea github]", resp.Providers)
	}
}

// ── auth middleware tests ─────────────────────────────────────────────────────

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/applications", nil, "")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuthMiddleware_InvalidJWT(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/applications", nil, "Bearer not.a.valid.jwt")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuthMiddleware_APIKeyAccepted(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodGet, "/applications", nil, "Bearer "+testAPIKey)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with valid API key", rr.Code)
	}
}

// ── user management ──────────────────────────────────────────────────────────

func adminHeaderFor(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	return userHeaderFor(t, userID, "admin@example.com", true)
}

func userHeaderFor(t *testing.T, userID uuid.UUID, email string, isAdmin bool) string {
	t.Helper()
	token, err := auth.GenerateToken(userID, email, isAdmin, testJWTSecret, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return "Bearer " + token
}

func TestDeleteUser_Success(t *testing.T) {
	st := newHandlerMockStore()
	target := &store.User{ID: uuid.New(), Email: "target@example.com"}
	st.users[target.Email] = target

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodDelete, "/users/"+target.ID.String(), nil, authHeader(t))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rr.Code, rr.Body)
	}
	if _, ok := st.users[target.Email]; ok {
		t.Error("expected user to be deleted from store")
	}
}

func TestDeleteUser_RejectsSelfDelete(t *testing.T) {
	st := newHandlerMockStore()
	self := &store.User{ID: uuid.New(), Email: "self@example.com", IsAdmin: true}
	st.users[self.Email] = self

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodDelete, "/users/"+self.ID.String(), nil, adminHeaderFor(t, self.ID))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body)
	}
	if _, ok := st.users[self.Email]; !ok {
		t.Error("user should not have been deleted")
	}
}

func TestDeleteUser_RejectsLastAdmin(t *testing.T) {
	st := newHandlerMockStore()
	lastAdmin := &store.User{ID: uuid.New(), Email: "lastadmin@example.com", IsAdmin: true}
	st.users[lastAdmin.Email] = lastAdmin

	router := newTestRouter(st)
	// Caller is a different admin identity than the target, so this exercises
	// the last-admin guard rather than the self-delete guard.
	rr := doRequest(t, router, http.MethodDelete, "/users/"+lastAdmin.ID.String(), nil, authHeader(t))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body)
	}
	if _, ok := st.users[lastAdmin.Email]; !ok {
		t.Error("last admin should not have been deleted")
	}
}

func TestResetUserPassword_Success(t *testing.T) {
	st := newHandlerMockStore()
	target := &store.User{ID: uuid.New(), Email: "target@example.com"}
	st.users[target.Email] = target

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost, "/users/"+target.ID.String()+"/password",
		map[string]string{"password": "newpassword123"}, authHeader(t))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rr.Code, rr.Body)
	}

	loginRR := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": target.Email, "password": "newpassword123"}, "")
	if loginRR.Code != http.StatusOK {
		t.Errorf("login with reset password: status = %d, want 200", loginRR.Code)
	}
}

func TestResetUserPassword_TooShort(t *testing.T) {
	st := newHandlerMockStore()
	target := &store.User{ID: uuid.New(), Email: "target@example.com"}
	st.users[target.Email] = target

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost, "/users/"+target.ID.String()+"/password",
		map[string]string{"password": "short"}, authHeader(t))
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestChangePassword_Success(t *testing.T) {
	st := newHandlerMockStore()
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpassword"), bcrypt.MinCost)
	self := &store.User{ID: uuid.New(), Email: "self@example.com", PasswordHash: string(hash)}
	st.users[self.Email] = self

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPut, "/auth/password",
		map[string]string{"current_password": "oldpassword", "new_password": "newpassword123"},
		userHeaderFor(t, self.ID, self.Email, false))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rr.Code, rr.Body)
	}

	loginRR := doRequest(t, router, http.MethodPost, "/auth/login",
		map[string]string{"email": self.Email, "password": "newpassword123"}, "")
	if loginRR.Code != http.StatusOK {
		t.Errorf("login with new password: status = %d, want 200", loginRR.Code)
	}
}

func TestChangePassword_WrongCurrentPassword(t *testing.T) {
	st := newHandlerMockStore()
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpassword"), bcrypt.MinCost)
	self := &store.User{ID: uuid.New(), Email: "self@example.com", PasswordHash: string(hash)}
	st.users[self.Email] = self

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPut, "/auth/password",
		map[string]string{"current_password": "wrong", "new_password": "newpassword123"},
		userHeaderFor(t, self.ID, self.Email, false))
	// 400, not 401: the session is valid, only the submitted password is wrong.
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestSetUserAdmin_Promote(t *testing.T) {
	st := newHandlerMockStore()
	target := &store.User{ID: uuid.New(), Email: "target@example.com"}
	st.users[target.Email] = target

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPut, "/users/"+target.ID.String()+"/admin",
		map[string]bool{"is_admin": true}, authHeader(t))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rr.Code, rr.Body)
	}
	if !st.users[target.Email].IsAdmin {
		t.Error("expected user to be promoted to admin")
	}
}

func TestSetUserAdmin_Demote(t *testing.T) {
	st := newHandlerMockStore()
	target := &store.User{ID: uuid.New(), Email: "target@example.com", IsAdmin: true}
	other := &store.User{ID: uuid.New(), Email: "other@example.com", IsAdmin: true}
	st.users[target.Email] = target
	st.users[other.Email] = other

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPut, "/users/"+target.ID.String()+"/admin",
		map[string]bool{"is_admin": false}, authHeader(t))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body: %s", rr.Code, rr.Body)
	}
	if st.users[target.Email].IsAdmin {
		t.Error("expected user to be demoted")
	}
}

func TestSetUserAdmin_RejectsDemotingLastAdmin(t *testing.T) {
	st := newHandlerMockStore()
	lastAdmin := &store.User{ID: uuid.New(), Email: "lastadmin@example.com", IsAdmin: true}
	st.users[lastAdmin.Email] = lastAdmin

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPut, "/users/"+lastAdmin.ID.String()+"/admin",
		map[string]bool{"is_admin": false}, authHeader(t))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body)
	}
	if !st.users[lastAdmin.Email].IsAdmin {
		t.Error("last admin should not have been demoted")
	}
}

func TestSetUserAdmin_RejectsNonAdminCaller(t *testing.T) {
	st := newHandlerMockStore()
	target := &store.User{ID: uuid.New(), Email: "target@example.com"}
	st.users[target.Email] = target

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPut, "/users/"+target.ID.String()+"/admin",
		map[string]bool{"is_admin": true}, userHeaderFor(t, uuid.New(), "user@example.com", false))
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestChangePassword_RejectsAPIKey(t *testing.T) {
	router := newTestRouter(newHandlerMockStore())
	rr := doRequest(t, router, http.MethodPut, "/auth/password",
		map[string]string{"current_password": "x", "new_password": "newpassword123"},
		"Bearer "+testAPIKey)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestUpdateGroup_Rename(t *testing.T) {
	st := newHandlerMockStore()
	router := newTestRouter(st)

	createRR := doRequest(t, router, http.MethodPost, "/groups", map[string]string{"name": "old-name"}, authHeader(t))
	var created store.Group
	decodeJSON(t, createRR, &created)

	rr := doRequest(t, router, http.MethodPut, "/groups/"+created.ID.String(),
		map[string]string{"name": "new-name"}, authHeader(t))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body)
	}
	var updated store.Group
	decodeJSON(t, rr, &updated)
	if updated.Name != "new-name" {
		t.Errorf("Name = %q, want %q", updated.Name, "new-name")
	}
}

// ── override step tests ───────────────────────────────────────────────────────

func TestOverrideStep_RequiresReason(t *testing.T) {
	st := newHandlerMockStore()
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, Status: "failed"}

	router := newTestRouter(st)
	for _, body := range []any{nil, map[string]string{"reason": "   "}} {
		rr := doRequest(t, router, http.MethodPost,
			"/runs/"+runID.String()+"/steps/0/override", body, authHeader(t))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %v: status = %d, want 400 (reason is mandatory)", body, rr.Code)
		}
	}
}

func TestOverrideStep_RunNotFailed(t *testing.T) {
	st := newHandlerMockStore()
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, Status: "success"}

	router := newTestRouter(st)
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+runID.String()+"/steps/0/override",
		map[string]string{"reason": "known flaky smoke test"}, authHeader(t))
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestOverrideStep_StepNotFailed(t *testing.T) {
	st := newHandlerMockStore()
	appID := uuid.New()
	st.applications[appID] = &store.Application{ID: appID, Provider: "github"}
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, ApplicationID: appID, Status: "failed"}
	st.steps[runID] = []*store.StepResult{
		{ID: uuid.New(), RunID: runID, StepIndex: 0, Status: "success"},
	}

	router := newTestRouterWithProviders(st, map[string]provider.Provider{"github": nil})
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+runID.String()+"/steps/0/override",
		map[string]string{"reason": "irrelevant"}, authHeader(t))
	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestOverrideStep_RecordsAuditTrailAndRequeues(t *testing.T) {
	st := newHandlerMockStore()
	appID := uuid.New()
	st.applications[appID] = &store.Application{ID: appID, Provider: "github"}
	runID := uuid.New()
	st.runs[runID] = &store.PipelineRun{ID: runID, ApplicationID: appID, Status: "failed"}
	st.steps[runID] = []*store.StepResult{
		{ID: uuid.New(), RunID: runID, StepIndex: 0, Status: "success"},
		{ID: uuid.New(), RunID: runID, StepIndex: 1, StepName: "dispatch_workflow:deploy.yml",
			Status: "failed", ErrorMessage: "workflow deploy.yml completed with conclusion: failure"},
		{ID: uuid.New(), RunID: runID, StepIndex: 2, Status: "skipped"},
	}

	router := newTestRouterWithProviders(st, map[string]provider.Provider{"github": nil})
	rr := doRequest(t, router, http.MethodPost,
		"/runs/"+runID.String()+"/steps/1/override",
		map[string]string{"reason": "deploy succeeded; only the post-deploy smoke test flaked"}, authHeader(t))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rr.Code, rr.Body)
	}

	overridden := st.steps[runID][1]
	if overridden.Status != "overridden" {
		t.Errorf("step status = %q, want overridden", overridden.Status)
	}
	if overridden.OverriddenBy != "test@example.com" {
		t.Errorf("overridden_by = %q, want the JWT identity", overridden.OverriddenBy)
	}
	if overridden.OverrideReason != "deploy succeeded; only the post-deploy smoke test flaked" {
		t.Errorf("override_reason = %q", overridden.OverrideReason)
	}
	if overridden.ErrorMessage == "" {
		t.Error("original failure message must be kept for the audit trail")
	}
}
