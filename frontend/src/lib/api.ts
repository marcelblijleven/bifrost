import { goto } from '$app/navigation';
import type { Application, PipelineRun, StepResult, ApprovalRequest, DashboardStats, User, Group } from './types';

// Same-origin in production (the Go binary serves both UI and API) and in
// dev (vite proxies /api to the Go backend). Auth rides along automatically
// as an httpOnly session cookie; JS never sees the token.
const API_URL = '/api';

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

async function req<T>(
	fetch: typeof globalThis.fetch,
	method: string,
	path: string,
	body?: unknown
): Promise<T> {
	const res = await fetch(`${API_URL}${path}`, {
		method,
		headers: { 'Content-Type': 'application/json' },
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
	if (!res.ok) {
		// 401 means the session cookie is missing, expired, or no longer
		// valid (e.g. rotated JWT secret, deleted user). /auth/login 401s on
		// bad credentials and /auth/me is the layout's own session probe;
		// both handle it themselves.
		if (res.status === 401 && path !== '/auth/login' && path !== '/auth/me') {
			goto('/login');
		}
		const text = await res.text().catch(() => res.statusText);
		throw new ApiError(res.status, `${method} ${path} → ${res.status}: ${text}`);
	}
	if (res.status === 204) return undefined as T;
	return res.json() as Promise<T>;
}

export function createApi(fetch: typeof globalThis.fetch) {
	return {
		setup: {
			status: () => req<{ needs_setup: boolean }>(fetch, 'GET', '/setup'),
			complete: (email: string, password: string) =>
				req<{ email: string }>(fetch, 'POST', '/setup', { email, password })
		},
		dashboard: {
			get: () => req<DashboardStats>(fetch, 'GET', '/dashboard')
		},
		providers: {
			list: () => req<{ providers: string[] }>(fetch, 'GET', '/providers')
		},
		auth: {
			login: (email: string, password: string) =>
				req<{ token: string }>(fetch, 'POST', '/auth/login', { email, password }),
			logout: () => req<void>(fetch, 'POST', '/auth/logout'),
			me: () => req<{ user_id: string; email: string; is_admin: boolean }>(fetch, 'GET', '/auth/me'),
			changePassword: (currentPassword: string, newPassword: string) =>
				req<void>(fetch, 'PUT', '/auth/password',
					{ current_password: currentPassword, new_password: newPassword })
		},
		applications: {
			list: () => req<Application[]>(fetch, 'GET', '/applications'),
			get: (id: string) => req<Application>(fetch, 'GET', `/applications/${id}`),
			create: (data: Partial<Application>) =>
				req<Application>(fetch, 'POST', '/applications', data),
			update: (id: string, data: Partial<Application>) =>
				req<Application>(fetch, 'PUT', `/applications/${id}`, data),
			delete: (id: string) => req<void>(fetch, 'DELETE', `/applications/${id}`),
			listRuns: (id: string, limit = 20, offset = 0, status = '', branch = '') =>
				req<PipelineRun[]>(fetch, 'GET',
					`/applications/${id}/runs?limit=${limit}&offset=${offset}&status=${encodeURIComponent(status)}&branch=${encodeURIComponent(branch)}`),
			installWebhook: (id: string) =>
				req<{ webhook_url: string }>(fetch, 'POST', `/applications/${id}/webhook/install`),
			acceptHead: (id: string, triggerRun = false) =>
				req<{ head: string; head_state: string; run_id?: string }>(
					fetch, 'POST', `/applications/${id}/head/accept`, { trigger_run: triggerRun }),
			listGroups: (id: string) =>
				req<Group[]>(fetch, 'GET', `/applications/${id}/groups`),
			grantGroup: (id: string, groupId: string) =>
				req<void>(fetch, 'PUT', `/applications/${id}/groups/${groupId}`),
			revokeGroup: (id: string, groupId: string) =>
				req<void>(fetch, 'DELETE', `/applications/${id}/groups/${groupId}`)
		},
		users: {
			list: () => req<User[]>(fetch, 'GET', '/users'),
			create: (email: string, password: string) =>
				req<User>(fetch, 'POST', '/users', { email, password }),
			delete: (id: string) => req<void>(fetch, 'DELETE', `/users/${id}`),
			resetPassword: (id: string, newPassword: string) =>
				req<void>(fetch, 'POST', `/users/${id}/password`, { password: newPassword }),
			setAdmin: (id: string, isAdmin: boolean) =>
				req<void>(fetch, 'PUT', `/users/${id}/admin`, { is_admin: isAdmin })
		},
		groups: {
			list: () => req<Group[]>(fetch, 'GET', '/groups'),
			create: (name: string) => req<Group>(fetch, 'POST', '/groups', { name }),
			rename: (id: string, name: string) => req<Group>(fetch, 'PUT', `/groups/${id}`, { name }),
			delete: (id: string) => req<void>(fetch, 'DELETE', `/groups/${id}`),
			listMembers: (id: string) => req<User[]>(fetch, 'GET', `/groups/${id}/members`),
			addMember: (groupId: string, userId: string) =>
				req<void>(fetch, 'PUT', `/groups/${groupId}/members/${userId}`),
			removeMember: (groupId: string, userId: string) =>
				req<void>(fetch, 'DELETE', `/groups/${groupId}/members/${userId}`)
		},
		runs: {
			get: (id: string) => req<PipelineRun>(fetch, 'GET', `/runs/${id}`),
			retryStep: (id: string, stepIndex: number) =>
				req<{ run_id: string }>(fetch, 'POST', `/runs/${id}/steps/${stepIndex}/retry`),
			overrideStep: (id: string, stepIndex: number, reason: string) =>
				req<{ run_id: string }>(fetch, 'POST', `/runs/${id}/steps/${stepIndex}/override`, { reason }),
			listSteps: (id: string) => req<StepResult[]>(fetch, 'GET', `/runs/${id}/steps`),
			listApprovals: (id: string) =>
				req<ApprovalRequest[]>(fetch, 'GET', `/runs/${id}/approvals`),
			approve: (id: string, stepIndex: number, by?: string) =>
				req<void>(fetch, 'POST', `/runs/${id}/approvals/${stepIndex}/approve`, by ? { by } : undefined),
			reject: (id: string, stepIndex: number, by?: string) =>
				req<void>(fetch, 'POST', `/runs/${id}/approvals/${stepIndex}/reject`, by ? { by } : undefined),
			cancel: (id: string) =>
				req<{ run_id: string }>(fetch, 'POST', `/runs/${id}/cancel`)
		}
	};
}

// Ready-made instance for components and event handlers; load functions
// should call createApi(fetch) with SvelteKit's fetch instead.
export const api = createApi((...args) => globalThis.fetch(...args));
