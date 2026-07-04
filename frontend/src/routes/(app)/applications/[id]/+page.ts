import type { PageLoad } from './$types';
import { createApi } from '$lib/api';
import { error } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, params, url }) => {
	const page = Math.max(1, Number(url.searchParams.get('page') ?? '1'));
	const limit = 20;
	const offset = (page - 1) * limit;
	const status = url.searchParams.get('status') ?? '';
	const branch = url.searchParams.get('branch') ?? '';

	const api = createApi(fetch);

	const [application, runs] = await Promise.all([
		api.applications.get(params.id).catch(() => { error(404, 'Application not found'); }),
		api.applications.listRuns(params.id, limit, offset, status, branch).catch(() => [] as never[])
	]);

	return { application, runs, page, limit, status, branch };
};
