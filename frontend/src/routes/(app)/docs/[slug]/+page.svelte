<script lang="ts">
	import type { PageData } from './$types';
	let { data }: { data: PageData } = $props();
</script>

<div class="mx-auto max-w-3xl px-6 py-10">
	<a href="/docs" class="mb-6 flex items-center gap-1.5 text-xs text-zinc-400 dark:text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300">
		<svg class="h-3.5 w-3.5" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
			<path stroke-linecap="round" stroke-linejoin="round" d="M15 19l-7-7 7-7" />
		</svg>
		Documentation
	</a>

	<article class="prose">
		{@html data.html}
	</article>
</div>

<style>
	:global(.prose h1) {
		@apply mb-2 text-2xl font-bold text-zinc-900 dark:text-zinc-100;
	}
	:global(.prose h2) {
		@apply mb-3 mt-10 border-t border-zinc-200 dark:border-zinc-800 pt-8 text-lg font-semibold text-zinc-900 dark:text-zinc-100;
	}
	:global(.prose h3) {
		@apply mb-2 mt-6 text-base font-semibold text-zinc-800 dark:text-zinc-200;
	}
	:global(.prose p) {
		@apply mb-4 text-sm leading-relaxed text-zinc-700 dark:text-zinc-300;
	}
	:global(.prose ul),
	:global(.prose ol) {
		@apply mb-4 space-y-1 pl-5 text-sm text-zinc-700 dark:text-zinc-300;
	}
	:global(.prose li) {
		@apply list-disc leading-relaxed;
	}
	:global(.prose ol li) {
		@apply list-decimal;
	}
	:global(.prose code) {
		@apply rounded bg-zinc-100 dark:bg-zinc-800 px-1.5 py-0.5 font-mono text-xs text-zinc-800 dark:text-zinc-200;
	}
	:global(.prose pre) {
		@apply mb-4 overflow-x-auto rounded-lg bg-zinc-50 dark:bg-zinc-900 p-4 text-xs leading-relaxed;
	}
	:global(.prose pre code) {
		@apply bg-transparent p-0 text-zinc-700 dark:text-zinc-300;
	}
	:global(.prose table) {
		@apply mb-4 w-full text-sm;
	}
	:global(.prose thead) {
		@apply border-b border-zinc-300 dark:border-zinc-700;
	}
	:global(.prose th) {
		@apply pb-2 pr-4 text-left text-xs font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400;
	}
	:global(.prose td) {
		@apply py-2 pr-4 align-top text-zinc-700 dark:text-zinc-300;
	}
	:global(.prose tr) {
		@apply border-b border-zinc-200/60 dark:border-zinc-800/60;
	}
	:global(.prose a) {
		@apply text-blue-500 dark:text-blue-400 underline hover:text-blue-600 dark:hover:text-blue-300;
	}
	:global(.prose hr) {
		@apply my-8 border-zinc-200 dark:border-zinc-800;
	}
	:global(.prose blockquote) {
		@apply border-l-2 border-zinc-300 dark:border-zinc-600 pl-4 italic text-zinc-500 dark:text-zinc-400;
	}
</style>
