import type { PageServerLoad, Actions } from './$types';
import { createApi } from '$lib/api';
import { error, fail, isRedirect } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ locals, fetch, params }) => {
	const api = createApi(fetch, locals.token);
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

export const actions: Actions = {
	rename: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const name = data.get('name')?.toString().trim() ?? '';
		if (!name) return fail(400, { error: 'Name is required' });
		const api = createApi(fetch, locals.token);
		try {
			await api.groups.rename(params.id, name);
		} catch (e: unknown) {
			return fail(422, { error: e instanceof Error ? e.message : 'Failed to rename group' });
		}
	},
	addMember: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const userId = data.get('userId')?.toString() ?? '';
		const api = createApi(fetch, locals.token);
		try {
			await api.groups.addMember(params.id, userId);
		} catch {
			return fail(500, { error: 'Failed to add member' });
		}
	},
	removeMember: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const userId = data.get('userId')?.toString() ?? '';
		const api = createApi(fetch, locals.token);
		try {
			await api.groups.removeMember(params.id, userId);
		} catch {
			return fail(500, { error: 'Failed to remove member' });
		}
	}
};
