import type { PageLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageLoad = async ({ fetch }) => {
	const api = createApi(fetch);
	const stats = await api.dashboard.get();
	return { stats };
};
