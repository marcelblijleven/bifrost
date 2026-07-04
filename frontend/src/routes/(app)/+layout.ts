import { redirect } from '@sveltejs/kit';
import { createApi, ApiError } from '$lib/api';
import type { LayoutLoad } from './$types';

// Session guard for all app pages. The token lives in an httpOnly cookie the
// browser attaches automatically, so "who am I" is a question for the API.
export const load: LayoutLoad = async ({ fetch }) => {
	const api = createApi(fetch);
	try {
		const user = await api.auth.me();
		return { user };
	} catch (e) {
		if (e instanceof ApiError && e.status === 401) {
			redirect(302, '/login');
		}
		throw e;
	}
};
