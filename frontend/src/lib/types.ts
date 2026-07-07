export interface StepConfig {
	type: string;
	config?: Record<string, unknown>;
}

export interface NotificationConfig {
	on_failure_url?: string;
	on_approval_url?: string;
	headers?: Record<string, string>;
}

export interface SkipConditions {
	commit_patterns?: string[];
	paths_ignore?: string[];
	paths_include?: string[];
	skip_backfill?: boolean;
}

export interface Application {
	ID: string;
	Name: string;
	Provider: string;
	Owner: string;
	Repo: string;
	Branch: string;
	WebhookSecret: string;
	PipelineSteps: StepConfig[];
	Notifications?: NotificationConfig;
	SkipConditions?: SkipConditions;
	/** 'push' runs on commits to Branch; 'tag' runs when a tag matching TagPattern is pushed. */
	TriggerType: 'push' | 'tag';
	/** Glob a pushed tag must match to trigger a tag-triggered application (e.g. "v*"). */
	TagPattern: string;
	/** Namespace for release tags created by the semver step (e.g. "frontend-" → frontend-v1.2.3). */
	TagPrefix: string;
	/** Head of the tracked branch as bifrost last saw it. */
	LastKnownSHA: string;
	/** 'ok', or 'blocked' after a force push / history rewrite until a human re-baselines the head. */
	HeadState: 'ok' | 'blocked';
	BlockedReason: string;
	BlockedAt: string | null;
	CreatedAt: string;
	UpdatedAt: string;
	/** Most recent pipeline run, present on list responses; null if never run. */
	LastRun?: PipelineRun | null;
}

export interface PipelineRun {
	ID: string;
	ApplicationID: string;
	CommitSHA: string;
	/** Branch head before the push that triggered this run; empty for API-triggered runs. */
	ParentSHA: string;
	CommitMessage: string;
	Branch: string;
	TriggeredBy: string;
	Status: 'pending' | 'running' | 'success' | 'failed' | 'cancelled' | 'superseded' | 'skipped' | 'blocked';
	Tag: string;
	/** Tag name that triggered this run; set only for tag-triggered applications. */
	TriggerTag: string;
	StartedAt: string | null;
	CompletedAt: string | null;
	CreatedAt: string;
}

export interface User {
	ID: string;
	Email: string;
	IsAdmin: boolean;
	CreatedAt: string;
}

export interface Group {
	ID: string;
	Name: string;
	CreatedAt: string;
}

export interface StepResult {
	ID: string;
	RunID: string;
	StepName: string;
	StepIndex: number;
	Status: 'pending' | 'running' | 'success' | 'failed' | 'skipped' | 'cancelled' | 'overridden';
	Output: string;
	ErrorMessage: string;
	ExternalRunID: number | null;
	/** Audit trail when a human overrode this failed step so the run could resume. */
	OverriddenBy: string;
	OverrideReason: string;
	OverriddenAt: string | null;
	StartedAt: string | null;
	CompletedAt: string | null;
}

export interface DayStats {
	date: string;
	total: number;
	succeeded: number;
	failed: number;
}

export interface RecentRun extends PipelineRun {
	application_name: string;
}

export interface PendingAction {
	run_id: string;
	application_id: string;
	application_name: string;
	type: 'approval' | 'queued';
	message: string;
	created_at: string;
}

export interface DashboardStats {
	total_runs: number;
	succeeded_runs: number;
	failed_runs: number;
	avg_duration_seconds: number;
	runs_by_day: DayStats[];
	recent_runs: RecentRun[];
	pending_actions: PendingAction[];
}

export interface ApprovalRequest {
	ID: string;
	RunID: string;
	StepName: string;
	StepIndex: number;
	Status: 'pending' | 'approved' | 'rejected' | 'superseded';
	ResolvedBy: string;
	Message: string;
	CreatedAt: string;
	ResolvedAt: string | null;
	SupersededBy: string | null;
}
