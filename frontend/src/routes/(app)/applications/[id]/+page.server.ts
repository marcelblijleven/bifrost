import type { PageServerLoad, Actions } from './$types';
import { createApi } from '$lib/api';
import { error, fail, redirect } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ locals, fetch, params, url }) => {
	const page = Math.max(1, Number(url.searchParams.get('page') ?? '1'));
	const limit = 20;
	const offset = (page - 1) * limit;
	const status = url.searchParams.get('status') ?? '';
	const branch = url.searchParams.get('branch') ?? '';

	const api = createApi(fetch, locals.token);

	const [application, runs] = await Promise.all([
		api.applications.get(params.id).catch(() => { error(404, 'Application not found'); }),
		api.applications.listRuns(params.id, limit, offset, status, branch).catch(() => [] as never[])
	]);

	return { application, runs, page, limit, status, branch };
};

export const actions: Actions = {
	acceptHead: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const triggerRun = data.get('triggerRun') === 'true';

		const api = createApi(fetch, locals.token);
		let runId: string | undefined;
		try {
			const res = await api.applications.acceptHead(params.id, triggerRun);
			runId = res.run_id;
		} catch {
			return fail(500, { error: 'Failed to accept the current branch head' });
		}
		if (runId) {
			redirect(303, `/applications/${params.id}/runs/${runId}`);
		}
	}
};
