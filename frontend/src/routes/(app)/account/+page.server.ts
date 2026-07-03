import type { Actions } from './$types';
import { createApi } from '$lib/api';
import { fail } from '@sveltejs/kit';

export const actions: Actions = {
	changePassword: async ({ locals, fetch, request }) => {
		const data = await request.formData();
		const currentPassword = data.get('currentPassword')?.toString() ?? '';
		const newPassword = data.get('newPassword')?.toString() ?? '';
		const confirmPassword = data.get('confirmPassword')?.toString() ?? '';

		if (!currentPassword || !newPassword) {
			return fail(400, { error: 'Current and new password are required' });
		}
		if (newPassword.length < 8) {
			return fail(400, { error: 'New password must be at least 8 characters' });
		}
		if (newPassword !== confirmPassword) {
			return fail(400, { error: 'New passwords do not match' });
		}

		const api = createApi(fetch, locals.token);
		try {
			await api.auth.changePassword(currentPassword, newPassword);
		} catch (e: unknown) {
			return fail(422, { error: e instanceof Error ? e.message : 'Failed to change password' });
		}
		return { success: true };
	}
};
