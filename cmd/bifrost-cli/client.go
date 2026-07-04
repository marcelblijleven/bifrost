package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(url, token string) *Client {
	// The API lives under /api on the server; accept URLs with or without
	// the suffix so both "https://bifrost.example.com" and ".../api" work.
	base := strings.TrimRight(url, "/")
	base = strings.TrimSuffix(base, "/api")
	return &Client{
		baseURL: base + "/api",
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var bodyR io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyR = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyR)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil && resp.StatusCode != 204 {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// sse returns a channel that receives a signal on each SSE event.
// The channel is closed when the stream ends or ctx is cancelled.
func (c *Client) sse(ctx context.Context, path string) (<-chan struct{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	sseClient := &http.Client{Timeout: 0}
	resp, err := sseClient.Do(req)
	if err != nil {
		return nil, err
	}
	ch := make(chan struct{}, 8)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data:") {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	}()
	return ch, nil
}

func (c *Client) Login(ctx context.Context, email, password string) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	err := c.do(ctx, "POST", "/auth/login", map[string]string{"email": email, "password": password}, &out)
	return out.Token, err
}

func (c *Client) Me(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	return out, c.do(ctx, "GET", "/auth/me", nil, &out)
}

func (c *Client) ChangePassword(ctx context.Context, currentPassword, newPassword string) error {
	return c.do(ctx, "PUT", "/auth/password", map[string]string{
		"current_password": currentPassword,
		"new_password":     newPassword,
	}, nil)
}

func (c *Client) ListApplications(ctx context.Context) ([]Application, error) {
	var out []Application
	return out, c.do(ctx, "GET", "/applications", nil, &out)
}

func (c *Client) GetApplication(ctx context.Context, id string) (Application, error) {
	var out Application
	return out, c.do(ctx, "GET", "/applications/"+id, nil, &out)
}

func (c *Client) CreateApplication(ctx context.Context, body map[string]any) (Application, error) {
	var out Application
	return out, c.do(ctx, "POST", "/applications", body, &out)
}

func (c *Client) DeleteApplication(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/applications/"+id, nil, nil)
}

func (c *Client) ListRuns(ctx context.Context, appID, status, branch string, limit, offset int) ([]Run, error) {
	var out []Run
	path := fmt.Sprintf("/applications/%s/runs?limit=%d&offset=%d&status=%s&branch=%s",
		appID, limit, offset, status, branch)
	return out, c.do(ctx, "GET", path, nil, &out)
}

func (c *Client) GetRun(ctx context.Context, id string) (Run, error) {
	var out Run
	return out, c.do(ctx, "GET", "/runs/"+id, nil, &out)
}

func (c *Client) ListSteps(ctx context.Context, runID string) ([]StepResult, error) {
	var out []StepResult
	return out, c.do(ctx, "GET", "/runs/"+runID+"/steps", nil, &out)
}

func (c *Client) ListApprovals(ctx context.Context, runID string) ([]ApprovalRequest, error) {
	var out []ApprovalRequest
	return out, c.do(ctx, "GET", "/runs/"+runID+"/approvals", nil, &out)
}

func (c *Client) Approve(ctx context.Context, runID string, stepIndex int) error {
	return c.do(ctx, "POST", fmt.Sprintf("/runs/%s/approvals/%d/approve", runID, stepIndex), nil, nil)
}

func (c *Client) Reject(ctx context.Context, runID string, stepIndex int) error {
	return c.do(ctx, "POST", fmt.Sprintf("/runs/%s/approvals/%d/reject", runID, stepIndex), nil, nil)
}

func (c *Client) RetryStep(ctx context.Context, runID string, stepIndex int) error {
	return c.do(ctx, "POST", fmt.Sprintf("/runs/%s/steps/%d/retry", runID, stepIndex), nil, nil)
}

func (c *Client) CancelRun(ctx context.Context, runID string) error {
	return c.do(ctx, "POST", "/runs/"+runID+"/cancel", nil, nil)
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var out []User
	return out, c.do(ctx, "GET", "/users", nil, &out)
}

func (c *Client) CreateUser(ctx context.Context, email, password string) (User, error) {
	var out User
	return out, c.do(ctx, "POST", "/users", map[string]string{"email": email, "password": password}, &out)
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/users/"+id, nil, nil)
}

func (c *Client) ResetUserPassword(ctx context.Context, id, newPassword string) error {
	return c.do(ctx, "POST", "/users/"+id+"/password", map[string]string{"password": newPassword}, nil)
}

func (c *Client) SetUserAdmin(ctx context.Context, id string, isAdmin bool) error {
	return c.do(ctx, "PUT", "/users/"+id+"/admin", map[string]bool{"is_admin": isAdmin}, nil)
}

func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	var out []Group
	return out, c.do(ctx, "GET", "/groups", nil, &out)
}

func (c *Client) CreateGroup(ctx context.Context, name string) (Group, error) {
	var out Group
	return out, c.do(ctx, "POST", "/groups", map[string]string{"name": name}, &out)
}

func (c *Client) UpdateGroup(ctx context.Context, id, name string) (Group, error) {
	var out Group
	return out, c.do(ctx, "PUT", "/groups/"+id, map[string]string{"name": name}, &out)
}

func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/groups/"+id, nil, nil)
}

func (c *Client) ListGroupMembers(ctx context.Context, groupID string) ([]User, error) {
	var out []User
	return out, c.do(ctx, "GET", "/groups/"+groupID+"/members", nil, &out)
}

func (c *Client) AddGroupMember(ctx context.Context, groupID, userID string) error {
	return c.do(ctx, "PUT", "/groups/"+groupID+"/members/"+userID, nil, nil)
}

func (c *Client) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	return c.do(ctx, "DELETE", "/groups/"+groupID+"/members/"+userID, nil, nil)
}

func (c *Client) GetDashboard(ctx context.Context) (DashboardStats, error) {
	var out DashboardStats
	return out, c.do(ctx, "GET", "/dashboard", nil, &out)
}
