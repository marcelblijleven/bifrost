import type { PageServerLoad, Actions } from './$types';
import { createApi } from '$lib/api';
import { fail } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ locals, fetch }) => {
	const api = createApi(fetch, locals.token);
	const [users, groups] = await Promise.all([
		api.users.list().catch(() => []),
		api.groups.list().catch(() => []),
	]);
	return { users, groups };
};

export const actions: Actions = {
	createUser: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const email = data.get('email')?.toString().trim() ?? '';
		const password = data.get('password')?.toString() ?? '';
		if (!email || !password) return fail(400, { error: 'Email and password are required', tab: 'users' as const });
		const api = createApi(fetch, locals.token);
		try {
			await api.users.create(email, password);
		} catch (e: unknown) {
			return fail(500, { error: e instanceof Error ? e.message : 'Failed to create user', tab: 'users' as const });
		}
	},
	createGroup: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const name = data.get('name')?.toString().trim() ?? '';
		if (!name) return fail(400, { error: 'Name is required', tab: 'groups' as const });
		const api = createApi(fetch, locals.token);
		try {
			await api.groups.create(name);
		} catch {
			return fail(500, { error: 'Failed to create group', tab: 'groups' as const });
		}
	},
	deleteGroup: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const groupId = data.get('groupId')?.toString() ?? '';
		const api = createApi(fetch, locals.token);
		try {
			await api.groups.delete(groupId);
		} catch {
			return fail(500, { error: 'Failed to delete group', tab: 'groups' as const });
		}
	},
	deleteUser: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const userId = data.get('userId')?.toString() ?? '';
		const api = createApi(fetch, locals.token);
		try {
			await api.users.delete(userId);
		} catch (e: unknown) {
			return fail(422, { error: e instanceof Error ? e.message : 'Failed to delete user', tab: 'users' as const });
		}
	},
	setAdmin: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const userId = data.get('userId')?.toString() ?? '';
		const isAdmin = data.get('isAdmin')?.toString() === 'true';
		const api = createApi(fetch, locals.token);
		try {
			await api.users.setAdmin(userId, isAdmin);
		} catch (e: unknown) {
			return fail(422, { error: e instanceof Error ? e.message : 'Failed to update admin rights', tab: 'users' as const });
		}
	},
	resetPassword: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const userId = data.get('userId')?.toString() ?? '';
		const newPassword = data.get('newPassword')?.toString() ?? '';
		if (!newPassword || newPassword.length < 8) {
			return fail(400, { error: 'Password must be at least 8 characters', tab: 'users' as const });
		}
		const api = createApi(fetch, locals.token);
		try {
			await api.users.resetPassword(userId, newPassword);
		} catch (e: unknown) {
			return fail(422, { error: e instanceof Error ? e.message : 'Failed to reset password', tab: 'users' as const });
		}
	},
};
