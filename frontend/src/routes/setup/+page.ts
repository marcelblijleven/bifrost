import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageLoad = async ({ fetch }) => {
	const api = createApi(fetch);

	const me = await api.auth.me().catch(() => null);
	if (me) {
		redirect(302, '/');
	}

	const { needs_setup } = await api.setup.status().catch(() => ({ needs_setup: false }));
	if (!needs_setup) {
		redirect(302, '/login');
	}
};
