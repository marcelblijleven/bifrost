import { error } from '@sveltejs/kit';
import { marked } from 'marked';
import { getDocContent, docManifest } from '$lib/docs';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => {
	const meta = docManifest.find((d) => d.slug === params.slug);
	if (!meta) throw error(404, 'Documentation not found');

	const raw = getDocContent(params.slug);
	if (!raw) throw error(404, 'Documentation not found');

	const html = await marked(raw);
	return { meta, html };
};
