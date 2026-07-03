import type { PageServerLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageServerLoad = async ({ locals, fetch }) => {
	const api = createApi(fetch, locals.token);
	const applications = await api.applications.list();

	return { applications };
};
