package main

import "time"

type Application struct {
	ID            string    `json:"ID"`
	Name          string    `json:"Name"`
	Provider      string    `json:"Provider"`
	Owner         string    `json:"Owner"`
	Repo          string    `json:"Repo"`
	Branch        string    `json:"Branch"`
	WebhookSecret string    `json:"WebhookSecret"`
	CreatedAt     time.Time `json:"CreatedAt"`
}

type Run struct {
	ID            string     `json:"ID"`
	ApplicationID string     `json:"ApplicationID"`
	CommitSHA     string     `json:"CommitSHA"`
	CommitMessage string     `json:"CommitMessage"`
	Branch        string     `json:"Branch"`
	TriggeredBy   string     `json:"TriggeredBy"`
	Status        string     `json:"Status"`
	Tag           string     `json:"Tag"`
	StartedAt     *time.Time `json:"StartedAt"`
	CompletedAt   *time.Time `json:"CompletedAt"`
	CreatedAt     time.Time  `json:"CreatedAt"`
}

type StepResult struct {
	ID            string     `json:"ID"`
	StepName      string     `json:"StepName"`
	StepIndex     int        `json:"StepIndex"`
	Status        string     `json:"Status"`
	Output        string     `json:"Output"`
	ErrorMessage  string     `json:"ErrorMessage"`
	ExternalRunID *int64     `json:"ExternalRunID"`
	StartedAt     *time.Time `json:"StartedAt"`
	CompletedAt   *time.Time `json:"CompletedAt"`
}

type ApprovalRequest struct {
	ID        string `json:"ID"`
	StepName  string `json:"StepName"`
	StepIndex int    `json:"StepIndex"`
	Status    string `json:"Status"`
	Message   string `json:"Message"`
}

type User struct {
	ID        string    `json:"ID"`
	Email     string    `json:"Email"`
	IsAdmin   bool      `json:"IsAdmin"`
	CreatedAt time.Time `json:"CreatedAt"`
}

type Group struct {
	ID        string    `json:"ID"`
	Name      string    `json:"Name"`
	CreatedAt time.Time `json:"CreatedAt"`
}

type DashboardStats struct {
	TotalRuns          int     `json:"total_runs"`
	SucceededRuns      int     `json:"succeeded_runs"`
	FailedRuns         int     `json:"failed_runs"`
	AvgDurationSeconds float64 `json:"avg_duration_seconds"`
	PendingActions     []struct {
		ApplicationName string `json:"application_name"`
		Type            string `json:"type"`
		Message         string `json:"message"`
	} `json:"pending_actions"`
}
