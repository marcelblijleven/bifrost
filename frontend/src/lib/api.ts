import { redirect } from '@sveltejs/kit';
import type { Application, PipelineRun, StepResult, ApprovalRequest, DashboardStats, User, Group } from './types';

const API_URL = process.env.API_URL ?? 'http://localhost:8080';

export class ApiError extends Error {
	status: number;
	constructor(status: number, message: string) {
		super(message);
		this.status = status;
	}
}

function headers(token?: string): Record<string, string> {
	const h: Record<string, string> = { 'Content-Type': 'application/json' };
	if (token) h['Authorization'] = `Bearer ${token}`;
	return h;
}

async function req<T>(
	fetch: typeof globalThis.fetch,
	method: string,
	path: string,
	token?: string,
	body?: unknown
): Promise<T> {
	const res = await fetch(`${API_URL}${path}`, {
		method,
		headers: headers(token),
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
	if (!res.ok) {
		// 401 on an authenticated call means the session is no longer valid
		// (e.g. rotated JWT secret, deleted user) — token expiry itself is
		// already handled by hooks.server.ts before we get here. The backend
		// never uses 401 for anything but authentication on these routes.
		if (res.status === 401 && token) {
			redirect(303, '/login');
		}
		const text = await res.text().catch(() => res.statusText);
		throw new ApiError(res.status, `${method} ${path} → ${res.status}: ${text}`);
	}
	if (res.status === 204) return undefined as T;
	return res.json() as Promise<T>;
}

export function createApi(fetch: typeof globalThis.fetch, token?: string) {
	return {
		setup: {
			status: () => req<{ needs_setup: boolean }>(fetch, 'GET', '/setup'),
			complete: (email: string, password: string) =>
				req<{ email: string }>(fetch, 'POST', '/setup', undefined, { email, password })
		},
		dashboard: {
			get: () => req<DashboardStats>(fetch, 'GET', '/dashboard', token)
		},
		providers: {
			list: () => req<{ providers: string[] }>(fetch, 'GET', '/providers', token)
		},
		auth: {
			login: (email: string, password: string) =>
				req<{ token: string }>(fetch, 'POST', '/auth/login', undefined, { email, password }),
			me: () => req<{ user_id: string; email: string }>(fetch, 'GET', '/auth/me', token),
			changePassword: (currentPassword: string, newPassword: string) =>
				req<void>(fetch, 'PUT', '/auth/password', token,
					{ current_password: currentPassword, new_password: newPassword })
		},
		applications: {
			list: () => req<Application[]>(fetch, 'GET', '/applications', token),
			get: (id: string) => req<Application>(fetch, 'GET', `/applications/${id}`, token),
			create: (data: Partial<Application>) =>
				req<Application>(fetch, 'POST', '/applications', token, data),
			update: (id: string, data: Partial<Application>) =>
				req<Application>(fetch, 'PUT', `/applications/${id}`, token, data),
			delete: (id: string) => req<void>(fetch, 'DELETE', `/applications/${id}`, token),
			listRuns: (id: string, limit = 20, offset = 0, status = '', branch = '') =>
				req<PipelineRun[]>(fetch, 'GET',
					`/applications/${id}/runs?limit=${limit}&offset=${offset}&status=${encodeURIComponent(status)}&branch=${encodeURIComponent(branch)}`,
					token),
			installWebhook: (id: string) =>
				req<{ webhook_url: string }>(fetch, 'POST', `/applications/${id}/webhook/install`, token),
			acceptHead: (id: string, triggerRun = false) =>
				req<{ head: string; head_state: string; run_id?: string }>(
					fetch, 'POST', `/applications/${id}/head/accept`, token, { trigger_run: triggerRun }),
			listGroups: (id: string) =>
				req<Group[]>(fetch, 'GET', `/applications/${id}/groups`, token),
			grantGroup: (id: string, groupId: string) =>
				req<void>(fetch, 'PUT', `/applications/${id}/groups/${groupId}`, token),
			revokeGroup: (id: string, groupId: string) =>
				req<void>(fetch, 'DELETE', `/applications/${id}/groups/${groupId}`, token)
		},
		users: {
			list: () => req<User[]>(fetch, 'GET', '/users', token),
			create: (email: string, password: string) =>
				req<User>(fetch, 'POST', '/users', token, { email, password }),
			delete: (id: string) => req<void>(fetch, 'DELETE', `/users/${id}`, token),
			resetPassword: (id: string, newPassword: string) =>
				req<void>(fetch, 'POST', `/users/${id}/password`, token, { password: newPassword }),
			setAdmin: (id: string, isAdmin: boolean) =>
				req<void>(fetch, 'PUT', `/users/${id}/admin`, token, { is_admin: isAdmin })
		},
		groups: {
			list: () => req<Group[]>(fetch, 'GET', '/groups', token),
			create: (name: string) => req<Group>(fetch, 'POST', '/groups', token, { name }),
			rename: (id: string, name: string) => req<Group>(fetch, 'PUT', `/groups/${id}`, token, { name }),
			delete: (id: string) => req<void>(fetch, 'DELETE', `/groups/${id}`, token),
			listMembers: (id: string) => req<User[]>(fetch, 'GET', `/groups/${id}/members`, token),
			addMember: (groupId: string, userId: string) =>
				req<void>(fetch, 'PUT', `/groups/${groupId}/members/${userId}`, token),
			removeMember: (groupId: string, userId: string) =>
				req<void>(fetch, 'DELETE', `/groups/${groupId}/members/${userId}`, token)
		},
		runs: {
			get: (id: string) => req<PipelineRun>(fetch, 'GET', `/runs/${id}`, token),
			retryStep: (id: string, stepIndex: number) =>
				req<{ run_id: string }>(fetch, 'POST', `/runs/${id}/steps/${stepIndex}/retry`, token),
			overrideStep: (id: string, stepIndex: number, reason: string) =>
				req<{ run_id: string }>(fetch, 'POST', `/runs/${id}/steps/${stepIndex}/override`, token, { reason }),
			listSteps: (id: string) => req<StepResult[]>(fetch, 'GET', `/runs/${id}/steps`, token),
			listApprovals: (id: string) =>
				req<ApprovalRequest[]>(fetch, 'GET', `/runs/${id}/approvals`, token),
			approve: (id: string, stepIndex: number, by?: string) =>
				req<void>(fetch, 'POST', `/runs/${id}/approvals/${stepIndex}/approve`, token, by ? { by } : undefined),
			reject: (id: string, stepIndex: number, by?: string) =>
				req<void>(fetch, 'POST', `/runs/${id}/approvals/${stepIndex}/reject`, token, by ? { by } : undefined),
			cancel: (id: string) =>
				req<{ run_id: string }>(fetch, 'POST', `/runs/${id}/cancel`, token)
		}
	};
}
