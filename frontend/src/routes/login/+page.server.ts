import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { createApi, ApiError } from '$lib/api';

export const load: PageServerLoad = async ({ locals, fetch }) => {
	if (locals.user) {
		redirect(302, '/');
	}

	const api = createApi(fetch);
	const { needs_setup } = await api.setup.status().catch(() => ({ needs_setup: false }));
	if (needs_setup) {
		redirect(302, '/setup');
	}
};

export const actions: Actions = {
	default: async ({ request, cookies, fetch }) => {
		const data = await request.formData();
		const email = data.get('email') as string;
		const password = data.get('password') as string;

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required' });
		}

		try {
			const api = createApi(fetch);
			const { token } = await api.auth.login(email, password);

			cookies.set('token', token, {
				path: '/',
				httpOnly: true,
				sameSite: 'strict',
				secure: process.env.NODE_ENV === 'production',
				maxAge: 60 * 60 * 24
			});
		} catch (e: unknown) {
			if (e instanceof ApiError && e.status === 429) {
				return fail(429, { error: 'Too many failed attempts. Try again in a few minutes.' });
			}
			return fail(401, { error: 'Invalid email or password' });
		}

		redirect(302, '/');
	}
};
