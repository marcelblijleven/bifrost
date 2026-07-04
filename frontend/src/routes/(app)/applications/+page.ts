import type { PageLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageLoad = async ({ fetch }) => {
	const api = createApi(fetch);
	const applications = await api.applications.list();

	return { applications };
};
