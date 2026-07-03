import type { PageServerLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageServerLoad = async ({ locals, fetch }) => {
	const api = createApi(fetch, locals.token);
	const stats = await api.dashboard.get();
	return { stats };
};
