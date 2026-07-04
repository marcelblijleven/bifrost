import type { PageLoad } from './$types';
import { createApi } from '$lib/api';
import { error, isRedirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, params }) => {
	const api = createApi(fetch);
	const [groups, members, users] = await Promise.all([
		api.groups.list().catch((e) => {
			if (isRedirect(e)) throw e;
			return [];
		}),
		api.groups.listMembers(params.id).catch(() => []),
		api.users.list().catch(() => [])
	]);
	const group = groups.find((g) => g.ID === params.id);
	if (!group) error(404, 'Group not found');
	return { group, members, users };
};
