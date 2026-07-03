export interface DocMeta {
	slug: string;
	title: string;
	description: string;
}

export const docManifest: DocMeta[] = [
	{
		slug: 'getting-started',
		title: 'Getting started',
		description: 'Installation, environment variables, and your first application'
	},
	{
		slug: 'pipeline-steps',
		title: 'Pipeline steps',
		description: 'Reference for all available step types and skip conditions'
	},
	{
		slug: 'webhooks',
		title: 'Webhooks',
		description: 'Setting up GitHub webhooks to trigger pipelines'
	},
	{
		slug: 'deployment',
		title: 'Production deployment',
		description: 'Running Bifrost in production with LXC, systemd, nginx, and TLS'
	},
	{
		slug: 'api',
		title: 'API reference',
		description: 'REST API endpoints and authentication'
	}
];

// Eagerly bundled at build time by Vite.
const rawDocs = import.meta.glob('./docs/*.md', {
	query: '?raw',
	import: 'default',
	eager: true
}) as Record<string, string>;

export function getDocContent(slug: string): string | null {
	return rawDocs[`./docs/${slug}.md`] ?? null;
}
