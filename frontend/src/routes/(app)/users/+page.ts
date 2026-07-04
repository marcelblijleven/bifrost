import type { PageLoad } from './$types';
import { createApi } from '$lib/api';

export const load: PageLoad = async ({ fetch }) => {
	const api = createApi(fetch);
	const [users, groups] = await Promise.all([
		api.users.list().catch(() => []),
		api.groups.list().catch(() => []),
	]);
	return { users, groups };
};
