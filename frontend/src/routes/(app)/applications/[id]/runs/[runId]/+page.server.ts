import type { PageServerLoad, Actions } from './$types';
import { createApi } from '$lib/api';
import { error, fail, redirect, isRedirect } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ locals, fetch, params }) => {
	const api = createApi(fetch, locals.token);

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

export const actions: Actions = {
	approve: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const stepIndex = Number(data.get('stepIndex'));

		if (isNaN(stepIndex)) return fail(400, { error: 'Invalid step index' });

		const api = createApi(fetch, locals.token);
		await api.runs.approve(params.runId, stepIndex, locals.user?.email).catch(() => {
			return fail(500, { error: 'Failed to approve step' });
		});
	},

	reject: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const stepIndex = Number(data.get('stepIndex'));

		if (isNaN(stepIndex)) return fail(400, { error: 'Invalid step index' });

		const api = createApi(fetch, locals.token);
		await api.runs.reject(params.runId, stepIndex, locals.user?.email).catch(() => {
			return fail(500, { error: 'Failed to reject step' });
		});
	},

	retry: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const stepIndex = Number(data.get('stepIndex'));
		if (isNaN(stepIndex)) return fail(400, { error: 'Invalid step index' });

		const api = createApi(fetch, locals.token);
		try {
			await api.runs.retryStep(params.runId, stepIndex);
		} catch {
			return fail(500, { error: 'Failed to retry step' });
		}
		redirect(303, `/applications/${params.id}/runs/${params.runId}`);
	},

	override: async ({ locals, fetch, params, request }) => {
		const data = await request.formData();
		const stepIndex = Number(data.get('stepIndex'));
		const reason = String(data.get('reason') ?? '').trim();
		if (isNaN(stepIndex)) return fail(400, { error: 'Invalid step index' });
		if (!reason) return fail(400, { error: 'A reason is required to override a failed step' });

		const api = createApi(fetch, locals.token);
		try {
			await api.runs.overrideStep(params.runId, stepIndex, reason);
		} catch {
			return fail(500, { error: 'Failed to override step' });
		}
	},

	cancel: async ({ locals, fetch, params }) => {
		const api = createApi(fetch, locals.token);
		try {
			await api.runs.cancel(params.runId);
		} catch {
			return fail(500, { error: 'Failed to cancel run' });
		}
	}
};
