import type { PageLoad } from './$types';
import { createApi } from '$lib/api';
import { error, isRedirect } from '@sveltejs/kit';

export const load: PageLoad = async ({ fetch, params }) => {
	const api = createApi(fetch);

	const [run, steps, approvals, application] = await Promise.all([
		api.runs.get(params.runId).catch((e) => {
			if (isRedirect(e)) throw e;
			error(404, 'Run not found');
		}),
		api.runs.listSteps(params.runId).catch(() => []),
		api.runs.listApprovals(params.runId).catch(() => []),
		api.applications.get(params.id).catch(() => null)
	]);

	return { run, steps, approvals, appId: params.id, application };
};
