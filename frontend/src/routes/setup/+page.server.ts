import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageServerLoad = async ({ locals, fetch }) => {
	if (locals.user) {
		redirect(302, '/');
	}

	const api = createApi(fetch);
	const { needs_setup } = await api.setup.status().catch(() => ({ needs_setup: false }));
	if (!needs_setup) {
		redirect(302, '/login');
	}
};

export const actions: Actions = {
	default: async ({ request, fetch }) => {
		const data = await request.formData();
		const email = data.get('email') as string;
		const password = data.get('password') as string;
		const confirm = data.get('confirm') as string;

		if (!email || !password || !confirm) {
			return fail(400, { error: 'All fields are required' });
		}
		if (password !== confirm) {
			return fail(400, { error: 'Passwords do not match' });
		}
		if (password.length < 8) {
			return fail(400, { error: 'Password must be at least 8 characters' });
		}

		try {
			const api = createApi(fetch);
			await api.setup.complete(email, password);
		} catch {
			return fail(500, { error: 'Failed to create account. Please try again.' });
		}

		redirect(303, '/login');
	}
};
